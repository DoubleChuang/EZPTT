// Package config resolves the runtime configuration.
//
// Precedence is flag (handled by main) > environment > config file > default.
// The config file is a plain KEY=VALUE file, so no viper dependency is needed.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	// LogLevel is "debug", "info", "warn", "error" or the legacy 0-3.
	LogLevel string
	// CronSpec is a robfig/cron spec; 5 fields (minute first) or 6 (seconds first).
	CronSpec string
	// AccountsFile is the path to the account CSV.
	AccountsFile string
	// MaxConcurrency caps how many accounts log in at the same time.
	MaxConcurrency int
	// LoginTimeout bounds one account's whole login.
	LoginTimeout time.Duration
	// ReadTimeout bounds the wait for the next screen.
	ReadTimeout time.Duration
	// Hold keeps the session open this long after a successful login.
	Hold time.Duration
	// InsecureTLS disables certificate verification (only for broken setups).
	InsecureTLS bool
	// RunOnStart logs every account in once right after startup, then follows
	// the cron spec. Handy in a container where a restart is the only way to
	// trigger a run.
	RunOnStart bool
	// WSURL is the PTT websocket endpoint.
	WSURL string
	// Origin is the Origin header term.ptt.cc expects.
	Origin string
}

// EnvPrefix is prepended to every environment variable this program reads.
const EnvPrefix = "EZPTT_"

// Defaults returns the built-in configuration.
func Defaults() Config {
	return Config{
		LogLevel:       "info",
		CronSpec:       "5 3 * * *", // every day at 03:05 local time
		AccountsFile:   "PttConfig.csv",
		MaxConcurrency: 4,
		LoginTimeout:   90 * time.Second,
		ReadTimeout:    30 * time.Second,
		Hold:           0,
		InsecureTLS:    false,
		RunOnStart:     false,
		WSURL:          "wss://ws.ptt.cc/bbs",
		Origin:         "https://term.ptt.cc",
	}
}

// Load builds the configuration from the optional config file and the
// environment. Warnings describe recoverable problems (e.g. a missing default
// config file); errors describe values that cannot be used.
func Load(path string) (Config, []string, error) {
	cfg := Defaults()
	var warnings []string

	switch {
	case path != "":
		values, err := parseFile(path)
		if err != nil {
			return Config{}, nil, err
		}
		if err := apply(&cfg, values, path); err != nil {
			return Config{}, nil, err
		}
	case fileExists(".env"):
		// Backwards compatibility: the old version always read ./.env.
		values, err := parseFile(".env")
		if err != nil {
			return Config{}, nil, fmt.Errorf(".env: %w", err)
		}
		if err := apply(&cfg, values, ".env"); err != nil {
			return Config{}, nil, err
		}
	}

	if err := apply(&cfg, envValues(), "environment"); err != nil {
		return Config{}, nil, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, nil, err
	}
	return cfg, warnings, nil
}

func (c Config) validate() error {
	fields := len(strings.Fields(c.CronSpec))
	if fields != 5 && fields != 6 {
		return fmt.Errorf("CRON_SPEC %q 必須是 5 欄（分 時 日 月 週）或 6 欄（秒 分 時 日 月 週）", c.CronSpec)
	}
	if c.MaxConcurrency < 1 || c.MaxConcurrency > 16 {
		return fmt.Errorf("MAX_CONCURRENCY 必須介於 1 和 16，目前是 %d", c.MaxConcurrency)
	}
	if c.LoginTimeout <= 0 {
		return errors.New("LOGIN_TIMEOUT 必須大於 0")
	}
	if c.ReadTimeout <= 0 {
		return errors.New("READ_TIMEOUT 必須大於 0")
	}
	if c.Hold < 0 {
		return errors.New("HOLD 不能是負數")
	}
	if strings.TrimSpace(c.AccountsFile) == "" {
		return errors.New("ACCOUNTS_FILE 不能是空的")
	}
	if c.WSURL == "" {
		return errors.New("WS_URL 不能是空的")
	}
	return nil
}

// apply copies recognised keys from values into cfg. Keys are accepted in both
// the bare form (LOG_LEVEL) and the prefixed form (EZPTT_LOG_LEVEL).
func apply(cfg *Config, values map[string]string, source string) error {
	for key, raw := range values {
		value := strings.TrimSpace(raw)
		key = strings.TrimPrefix(key, EnvPrefix)
		switch key {
		case "LOG_LEVEL":
			cfg.LogLevel = value
		case "CRON_SPEC":
			cfg.CronSpec = value
		case "ACCOUNTS_FILE":
			cfg.AccountsFile = value
		case "MAX_CONCURRENCY":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s: MAX_CONCURRENCY=%q 不是整數", source, value)
			}
			cfg.MaxConcurrency = n
		case "LOGIN_TIMEOUT":
			d, err := parseDuration(value)
			if err != nil {
				return fmt.Errorf("%s: LOGIN_TIMEOUT=%q 無法解析（用 90s 或 90）", source, value)
			}
			cfg.LoginTimeout = d
		case "READ_TIMEOUT":
			d, err := parseDuration(value)
			if err != nil {
				return fmt.Errorf("%s: READ_TIMEOUT=%q 無法解析（用 30s 或 30）", source, value)
			}
			cfg.ReadTimeout = d
		case "HOLD":
			d, err := parseDuration(value)
			if err != nil {
				return fmt.Errorf("%s: HOLD=%q 無法解析（用 30s 或 30）", source, value)
			}
			cfg.Hold = d
		case "INSECURE_TLS":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s: INSECURE_TLS=%q 不是 true/false", source, value)
			}
			cfg.InsecureTLS = b
		case "RUN_ON_START":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s: RUN_ON_START=%q 不是 true/false", source, value)
			}
			cfg.RunOnStart = b
		case "WS_URL":
			cfg.WSURL = value
		case "ORIGIN":
			cfg.Origin = value
		default:
			return fmt.Errorf("%s: 未知的設定鍵 %q", source, key)
		}
	}
	return nil
}

func parseDuration(value string) (time.Duration, error) {
	if n, err := strconv.Atoi(value); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	return time.ParseDuration(value)
}

func envValues() map[string]string {
	out := map[string]string{}
	for _, key := range []string{
		"LOG_LEVEL", "CRON_SPEC", "ACCOUNTS_FILE", "MAX_CONCURRENCY",
		"LOGIN_TIMEOUT", "READ_TIMEOUT", "HOLD", "INSECURE_TLS", "RUN_ON_START",
		"WS_URL", "ORIGIN",
	} {
		if v, ok := os.LookupEnv(EnvPrefix + key); ok {
			out[EnvPrefix+key] = v
		}
	}
	return out
}

func parseFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("第 %d 行不是 KEY=VALUE: %q", i+1, line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			return nil, fmt.Errorf("第 %d 行缺少鍵名", i+1)
		}
		out[key] = value
	}
	return out, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
