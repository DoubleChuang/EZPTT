// Command ezptt logs PTT accounts in on a schedule so the sessions never lapse.
//
// One binary, three modes: run the scheduler (default), run a single round
// (-once), or just check that PTT is reachable (-probe).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/DoubleChuang/EZPTT/internal/accounts"
	"github.com/DoubleChuang/EZPTT/internal/config"
	"github.com/DoubleChuang/EZPTT/internal/logx"
	"github.com/DoubleChuang/EZPTT/internal/screen"
	"github.com/DoubleChuang/EZPTT/internal/session"
	"github.com/DoubleChuang/EZPTT/internal/transport/ws"
)

// version is stamped at build time (goreleaser / -ldflags "-X main.version=…").
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("ezptt", flag.ContinueOnError)
	var (
		configPath   = fs.String("config", "", "設定檔路徑（KEY=VALUE）；預設讀 ./.env")
		accountsPath = fs.String("accounts", "", "帳號 CSV 路徑，覆蓋設定檔的 ACCOUNTS_FILE")
		once         = fs.Bool("once", false, "只登入一輪就結束，有帳號失敗時 exit 1")
		probeOnly    = fs.Bool("probe", false, "連上 PTT 印出前幾個畫面後結束")
		showVersion  = fs.Bool("version", false, "印出版本就結束")
	)
	fs.Usage = func() {
		out := fs.Output()
		_, _ = fmt.Fprintf(out, "EZPTT %s — 定時登入 PTT，讓帳號保持活躍\n\n用法:\n  ezptt [旗標]\n\n旗標:\n", version)
		fs.PrintDefaults()
		_, _ = fmt.Fprint(out, `
環境變數（前綴 EZPTT_）:
  LOG_LEVEL        debug/info/warn/error（或舊版 0-3），預設 info
  CRON_SPEC        cron 表示式，5 欄（分 時 日 月 週）或 6 欄（秒 …），預設 "5 3 * * *"
  ACCOUNTS_FILE    帳號 CSV，預設 PttConfig.csv
  MAX_CONCURRENCY  同時登入幾個帳號，預設 4
  LOGIN_TIMEOUT    單一帳號的登入逾時，預設 90s
  READ_TIMEOUT     等待下一個畫面的逾時，預設 30s
  HOLD             登入成功後保持連線的時間，預設 0
  INSECURE_TLS     設 true 可跳過 TLS 憑證驗證（不建議）
  WS_URL           websocket 端點，預設 wss://ws.ptt.cc/bbs
  ORIGIN           Origin header，預設 https://term.ptt.cc

範例:
  ezptt -probe      # 只檢查能不能連上 PTT
  ezptt -once       # 立刻登入一輪
  ezptt             # 依 CRON_SPEC 常駐
`)
	}
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *showVersion {
		fmt.Printf("ezptt %s\n", version)
		return nil
	}

	cfg, warnings, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger, err := logx.New(cfg.LogLevel, os.Stdout)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		logger.Warn(w)
	}
	if *accountsPath != "" {
		cfg.AccountsFile = *accountsPath
	}

	dialer := ws.NewDialer(ws.Options{
		URL:                cfg.WSURL,
		Origin:             cfg.Origin,
		InsecureSkipVerify: cfg.InsecureTLS,
		ReadTimeout:        cfg.ReadTimeout,
	})
	runner := session.NewRunner(dialer, logger, session.Options{
		LoginTimeout: cfg.LoginTimeout,
		Hold:         cfg.Hold,
		Concurrency:  cfg.MaxConcurrency,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *probeOnly {
		return runProbe(ctx, dialer, logger)
	}

	list, accountWarnings, err := accounts.Load(cfg.AccountsFile)
	if err != nil {
		if *once {
			return err
		}
		// Daemon mode stays up: the file is re-read before every run, so the
		// container can start before the account file is in place.
		logger.Error("帳號檔讀不到，排程照常啟動，每次執行前會重讀", "file", cfg.AccountsFile, "err", err)
	} else {
		for _, w := range accountWarnings {
			logger.Warn(w)
		}
		logger.Info("載入帳號", "count", len(list), "file", cfg.AccountsFile, "version", version)
	}

	if *once {
		return runOnce(ctx, runner, list, logger)
	}
	return serve(ctx, runner, cfg, logger)
}

// runProbe proves the transport works without touching any credential.
func runProbe(ctx context.Context, dialer session.Dialer, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := dialer.Dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	logger.Info("已連上 PTT，讀取登入畫面")
	for i := 0; i < 5; i++ {
		payload, err := conn.Read()
		if err != nil {
			return fmt.Errorf("讀取畫面失敗: %w", err)
		}
		rule, matched := screen.Classify(string(payload))
		logger.Info("畫面", "index", i, "rule", rule.Name, "matched", matched, "text", screen.Summary(string(payload), 160))
		if matched {
			logger.Info("連線與畫面辨識正常", "rule", rule.Name)
			return nil
		}
	}
	return errors.New("讀了 5 個畫面都沒有辨識出登入畫面，PTT 可能改版了")
}

func runOnce(ctx context.Context, runner *session.Runner, list []accounts.Account, logger *slog.Logger) error {
	results := runner.LoginAll(ctx, list)
	ok, failed := session.Summarise(results)
	logger.Info("本輪結果", "ok", ok, "failed", failed, "total", len(results))
	if failed > 0 {
		return fmt.Errorf("%d/%d 個帳號登入失敗", failed, len(results))
	}
	return nil
}

func serve(ctx context.Context, runner *session.Runner, cfg config.Config, logger *slog.Logger) error {
	var cronOptions []cron.Option
	if fields := strings.Fields(cfg.CronSpec); len(fields) == 6 {
		cronOptions = append(cronOptions, cron.WithSeconds())
	}
	scheduler := cron.New(cronOptions...)

	job := func() {
		// Re-read every run: replacing the CSV does not need a restart.
		list, warnings, err := accounts.Load(cfg.AccountsFile)
		if err != nil {
			logger.Error("讀取帳號失敗，本輪跳過", "file", cfg.AccountsFile, "err", err)
			return
		}
		for _, w := range warnings {
			logger.Warn(w)
		}
		logger.Info("開始登入", "accounts", len(list))
		results := runner.LoginAll(ctx, list)
		ok, failed := session.Summarise(results)
		logger.Info("本輪結果", "ok", ok, "failed", failed)
	}

	if _, err := scheduler.AddFunc(cfg.CronSpec, job); err != nil {
		return fmt.Errorf("CRON_SPEC %q 無效: %w", cfg.CronSpec, err)
	}
	scheduler.Start()
	logger.Info("排程啟動", "spec", cfg.CronSpec, "next", nextRun(scheduler), "concurrency", cfg.MaxConcurrency)

	if cfg.RunOnStart {
		logger.Info("RUN_ON_START=true，先跑一輪")
		go job()
	}

	<-ctx.Done()
	logger.Info("收到停止訊號，等待進行中的登入結束")
	<-scheduler.Stop().Done()
	logger.Info("已停止")
	return nil
}

func nextRun(scheduler *cron.Cron) string {
	entries := scheduler.Entries()
	if len(entries) == 0 {
		return "未知"
	}
	return entries[0].Next.Format(time.RFC3339)
}
