package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if got := len(strings.Fields(cfg.CronSpec)); got != 5 {
		t.Fatalf("default cron spec has %d fields", got)
	}
	if cfg.InsecureTLS {
		t.Fatal("TLS verification must be on by default")
	}
	if cfg.MaxConcurrency < 1 {
		t.Fatal("bad default concurrency")
	}
}

func TestLoadFileAndEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ezptt.env")
	content := strings.Join([]string{
		"# 註解",
		"LOG_LEVEL=warn",
		"CRON_SPEC='0 4 * * *'",
		"ACCOUNTS_FILE=/data/accounts.csv",
		"LOGIN_TIMEOUT=45s",
		"MAX_CONCURRENCY=2",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvPrefix+"CRON_SPEC", "30 2 * * *") // env beats the file

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.CronSpec != "30 2 * * *" {
		t.Fatalf("CronSpec = %q, env should win", cfg.CronSpec)
	}
	if cfg.AccountsFile != "/data/accounts.csv" {
		t.Fatalf("AccountsFile = %q", cfg.AccountsFile)
	}
	if cfg.LoginTimeout != 45*time.Second {
		t.Fatalf("LoginTimeout = %s", cfg.LoginTimeout)
	}
	if cfg.MaxConcurrency != 2 {
		t.Fatalf("MaxConcurrency = %d", cfg.MaxConcurrency)
	}
}

func TestLoadSecondGranularitySpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ezptt.env")
	if err := os.WriteFile(path, []byte("CRON_SPEC=0 5 3 * * *\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CronSpec != "0 5 3 * * *" {
		t.Fatalf("CronSpec = %q", cfg.CronSpec)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]string{
		"bad cron":     "CRON_SPEC=every day\n",
		"bad duration": "LOGIN_TIMEOUT=soon\n",
		"bad int":      "MAX_CONCURRENCY=lots\n",
		"bad bool":     "INSECURE_TLS=yes-please\n",
		"unknown key":  "PTT_PASSWORD=hunter2\n",
		"missing =":    "LOG_LEVEL\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ezptt.env")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Load(path); err == nil {
				t.Fatalf("expected an error for %q", content)
			}
		})
	}
}

func TestParseDurationAcceptsBareSeconds(t *testing.T) {
	d, err := parseDuration("90")
	if err != nil || d != 90*time.Second {
		t.Fatalf("parseDuration(90) = %s, %v", d, err)
	}
}

// The old version read ./.env implicitly; that must keep working.
func TestLoadFallsBackToDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("LOG_LEVEL=debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cfg, _, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Fatal("an explicitly requested config file must exist")
	}
}
