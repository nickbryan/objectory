package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/storage"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
)

func main() {
	ctx := context.Background()

	logger := slogutil.NewJSONLogger()
	server := httputil.NewServer(logger)

	dbConfig, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.ErrorContext(ctx, "Failed to parse postgres connection string config", slog.Any("error", err))
		return
	}

	dbConfig.ConnConfig.Tracer = &tracelog.TraceLog{
		Logger:   &pgxSlogAdapter{logger: logger},
		LogLevel: tracelog.LogLevelInfo,
	}

	pool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to connect to postgres database", slog.Any("error", err))
		return
	}

	defer pool.Close()

	databaseQueries := postgres.New(pool)
	identityRepository := storage.NewIdentityRepository(databaseQueries, time.Now)

	server.Register(
		iam.Endpoints(logger, uuidV4Generator{}, identityRepository)...,
	)

	server.Serve(ctx)
}

type uuidV4Generator struct{}

func (u uuidV4Generator) GenerateUUIDV4() ([16]byte, error) {
	next, err := uuid.NewRandom()
	if err != nil {
		return [16]byte{}, fmt.Errorf("creating new random uuid: %w", err)
	}

	return next, nil
}

type pgxSlogAdapter struct {
	logger *slog.Logger
}

func (a *pgxSlogAdapter) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]interface{}) {
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

	a.logger.LogAttrs(ctx, lvl, msg, attrs...)
}
