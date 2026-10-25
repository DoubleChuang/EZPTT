// Package logx builds the process logger.
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// ParseLevel accepts level names ("debug", "info", "warn", "error") and the
// legacy numeric values the previous version documented (0=debug … 3=error).
func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info", "1":
		return slog.LevelInfo, nil
	case "debug", "0":
		return slog.LevelDebug, nil
	case "warn", "warning", "2":
		return slog.LevelWarn, nil
	case "error", "3":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("未知的 LOG_LEVEL %q（可用 debug/info/warn/error 或 0-3）", value)
}

// New returns a text logger writing to w at the requested level.
func New(level string, w io.Writer) (*slog.Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler), nil
}
