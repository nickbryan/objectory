// Package pgxlog adapts an *slog.Logger to pgx's tracelog.Logger interface so
// pgx tracing output flows through the same structured logger as the rest of
// the application.
package pgxlog

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/tracelog"
)

// Adapter satisfies tracelog.Logger by translating pgx log levels to slog
// levels and forwarding the message to the embedded *slog.Logger.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter returns an Adapter that writes pgx tracing output to logger.
func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

// Log implements tracelog.Logger.
func (a *Adapter) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	attrs := make([]slog.Attr, 0, len(data))
	for k, v := range data {
		attrs = append(attrs, slog.Any(k, v))
	}

	var lvl slog.Level

	switch level {
	case tracelog.LogLevelNone:
		return
	case tracelog.LogLevelDebug:
		lvl = slog.LevelDebug
	case tracelog.LogLevelInfo:
		lvl = slog.LevelInfo
	case tracelog.LogLevelWarn:
		lvl = slog.LevelWarn
	case tracelog.LogLevelError:
		lvl = slog.LevelError
	default:
		lvl = slog.LevelError

		attrs = append(attrs, slog.Any("invalid_pgx_log_level", level))
	}

	a.logger.LogAttrs(ctx, lvl, msg, attrs...) //nolint:sloglint // forwarding pgx tracelog messages; dynamic msg intended.
}
