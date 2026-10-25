// Package session drives one PTT login: read a screen, classify it, type the
// answer it asks for.
//
// It depends only on the Conn and Dialer ports, so the whole state machine runs
// in a unit test against a scripted connection — no ptt.cc, no clock, no TLS.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/DoubleChuang/EZPTT/internal/accounts"
	"github.com/DoubleChuang/EZPTT/internal/screen"
)

// Conn is one PTT terminal connection. Reads return one screen chunk.
type Conn interface {
	Read() ([]byte, error)
	Write(p []byte) error
	Close() error
}

// Dialer opens a Conn. The account is not part of the port: PTT asks for the
// credentials after the connection is up.
type Dialer interface {
	Dial(ctx context.Context) (Conn, error)
}

// Options tunes the login loop.
type Options struct {
	// LoginTimeout bounds one account's whole login.
	LoginTimeout time.Duration
	// Hold keeps the connection open after a successful login.
	Hold time.Duration
	// MaxScreens stops a login that keeps showing unrecognised screens.
	MaxScreens int
	// Concurrency caps parallel logins in LoginAll.
	Concurrency int
}

func (o Options) normalised() Options {
	if o.LoginTimeout <= 0 {
		o.LoginTimeout = 90 * time.Second
	}
	if o.Hold < 0 {
		o.Hold = 0
	}
	if o.MaxScreens <= 0 {
		o.MaxScreens = 40
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	return o
}

// Result is the outcome of one account's login.
type Result struct {
	Account  string
	OK       bool
	Err      error
	Duration time.Duration
	// LastScreen is a trimmed copy of the last screen, for the failure report.
	LastScreen string
}

func (r Result) String() string {
	if r.OK {
		return fmt.Sprintf("%s ok in %s", r.Account, r.Duration.Round(time.Millisecond))
	}
	return fmt.Sprintf("%s failed: %v", r.Account, r.Err)
}

// sleepFunc waits for d unless ctx is cancelled. It is a field so tests can run
// the state machine without the PTT pacing delays.
type sleepFunc func(ctx context.Context, d time.Duration) error

// Runner logs accounts in.
type Runner struct {
	dialer Dialer
	log    *slog.Logger
	opts   Options
	sleep  sleepFunc
}

// NewRunner wires a dialer, a logger and the tuning options.
func NewRunner(dialer Dialer, log *slog.Logger, opts Options) *Runner {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Runner{dialer: dialer, log: log, opts: opts.normalised(), sleep: sleep}
}

// Login runs one complete login. It returns a Result; an error never escapes as
// a panic, a log.Fatal or a goroutine leak.
func (r *Runner) Login(ctx context.Context, account accounts.Account) (res Result) {
	start := time.Now()
	res = Result{Account: account.User}
	defer func() { res.Duration = time.Since(start) }()

	ctx, cancel := context.WithTimeout(ctx, r.opts.LoginTimeout)
	defer cancel()

	conn, err := r.dialer.Dial(ctx)
	if err != nil {
		res.Err = fmt.Errorf("連線失敗: %w", err)
		return res
	}
	defer func() {
		if err := conn.Close(); err != nil {
			r.log.Debug("關閉連線失敗", "account", account.User, "err", err)
		}
	}()

	for i := 0; i < r.opts.MaxScreens; i++ {
		if err := ctx.Err(); err != nil {
			res.Err = fmt.Errorf("登入逾時（%s）", r.opts.LoginTimeout)
			return res
		}
		payload, err := conn.Read()
		if err != nil {
			res.Err = fmt.Errorf("讀取畫面失敗: %w", err)
			return res
		}
		text := string(payload)
		res.LastScreen = screen.Summary(text, 160)
		rule, matched := screen.Classify(text)
		if !matched {
			r.log.Debug("未辨識的畫面", "account", account.User, "screen", res.LastScreen)
			continue
		}
		r.log.Debug("畫面", "account", account.User, "rule", rule.Name, "action", rule.Action.String())

		if err := r.sleep(ctx, rule.Delay); err != nil {
			res.Err = fmt.Errorf("登入逾時（%s）", r.opts.LoginTimeout)
			return res
		}

		switch rule.Action {
		case screen.Ignore:
			continue
		case screen.LoggedIn:
			res.OK = true
		case screen.Failed:
			res.Err = errors.New(rule.Reason)
		case screen.Credentials:
			if err := conn.Write([]byte(account.User)); err != nil {
				res.Err = fmt.Errorf("送出帳號失敗: %w", err)
				return res
			}
			if err := r.sleep(ctx, time.Second); err != nil {
				res.Err = fmt.Errorf("登入逾時（%s）", r.opts.LoginTimeout)
				return res
			}
			if err := conn.Write([]byte(account.Password)); err != nil {
				res.Err = fmt.Errorf("送出密碼失敗: %w", err)
				return res
			}
		case screen.AnyKey:
			if err := conn.Write(nil); err != nil {
				res.Err = fmt.Errorf("送出任意鍵失敗: %w", err)
				return res
			}
		case screen.Password:
			if err := conn.Write([]byte(account.Password)); err != nil {
				res.Err = fmt.Errorf("送回密碼失敗: %w", err)
				return res
			}
		case screen.Confirm:
			if err := conn.Write([]byte("y")); err != nil {
				res.Err = fmt.Errorf("送出確認失敗: %w", err)
				return res
			}
		case screen.Quit:
			if err := conn.Write([]byte("q")); err != nil {
				res.Err = fmt.Errorf("送出離開失敗: %w", err)
				return res
			}
		}

		if res.OK || res.Err != nil {
			break
		}
	}

	if res.OK {
		if err := r.sleep(ctx, r.opts.Hold); err != nil {
			res.OK = false
			res.Err = fmt.Errorf("登入後保持連線被中斷: %w", err)
			return res
		}
		return res
	}
	if res.Err == nil {
		res.Err = fmt.Errorf("登入未完成（讀了 %d 個畫面，最後畫面：%s）", r.opts.MaxScreens, res.LastScreen)
	}
	return res
}

// LoginAll logs every account in, at most Concurrency at a time. The results are
// in the same order as the input.
func (r *Runner) LoginAll(ctx context.Context, list []accounts.Account) []Result {
	results := make([]Result, len(list))
	sem := make(chan struct{}, r.opts.Concurrency)

	var wg sync.WaitGroup
	for i, account := range list {
		wg.Add(1)
		go func(i int, account accounts.Account) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = Result{Account: account.User, Err: ctx.Err()}
				return
			}
			res := r.Login(ctx, account)
			results[i] = res
			if res.OK {
				r.log.Info("登入成功", "account", res.Account, "took", res.Duration.Round(time.Millisecond))
			} else {
				r.log.Error("登入失敗", "account", res.Account, "err", res.Err, "last_screen", res.LastScreen)
			}
		}(i, account)
	}
	wg.Wait()
	return results
}

// Summarise counts successes and failures.
func Summarise(results []Result) (ok, failed int) {
	for _, res := range results {
		if res.OK {
			ok++
			continue
		}
		failed++
	}
	return ok, failed
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
