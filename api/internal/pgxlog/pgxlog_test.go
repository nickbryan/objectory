package pgxlog_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/tracelog"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/objectory/api/internal/pgxlog"
)

func TestAdapter_Log(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	cases := map[string]struct {
		level    tracelog.LogLevel
		msg      string
		data     map[string]any
		want     slogmem.RecordQuery
		wantSkip bool
	}{
		"none level is skipped": {
			level:    tracelog.LogLevelNone,
			msg:      "noisy query",
			wantSkip: true,
		},
		"debug maps to debug": {
			level: tracelog.LogLevelDebug,
			msg:   "executing query",
			data:  map[string]any{"sql": "SELECT 1"},
			want: slogmem.RecordQuery{
				Level:   slog.LevelDebug,
				Message: "executing query",
				Attrs:   map[string]slog.Value{"sql": slog.StringValue("SELECT 1")},
			},
		},
		"info maps to info": {
			level: tracelog.LogLevelInfo,
			msg:   "row returned",
			want: slogmem.RecordQuery{
				Level:   slog.LevelInfo,
				Message: "row returned",
			},
		},
		"warn maps to warn": {
			level: tracelog.LogLevelWarn,
			msg:   "slow query",
			want: slogmem.RecordQuery{
				Level:   slog.LevelWarn,
				Message: "slow query",
			},
		},
		"error maps to error": {
			level: tracelog.LogLevelError,
			msg:   "query failed",
			data:  map[string]any{"err": errBoom},
			want: slogmem.RecordQuery{
				Level:   slog.LevelError,
				Message: "query failed",
				Attrs:   map[string]slog.Value{"err": slog.AnyValue(errBoom)},
			},
		},
		"unknown level maps to error and includes invalid_pgx_log_level": {
			level: tracelog.LogLevel(99),
			msg:   "unknown",
			want: slogmem.RecordQuery{
				Level:   slog.LevelError,
				Message: "unknown",
				Attrs:   map[string]slog.Value{"invalid_pgx_log_level": slog.AnyValue(tracelog.LogLevel(99))},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			logger, records := slogutil.NewInMemoryLogger(slog.LevelDebug)
			adapter := pgxlog.NewAdapter(logger)

			adapter.Log(context.Background(), tc.level, tc.msg, tc.data)

			if tc.wantSkip {
				if !records.IsEmpty() {
					t.Errorf("expected no records for level None, got %d", records.Len())
				}

				return
			}

			if ok, diff := records.Contains(tc.want); !ok {
				t.Errorf("expected record %+v\n%s", tc.want, diff)
			}
		})
	}
}
