// Package main is the entrypoint for the objectory API server.
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
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/storage"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
)

const (
	// minJWTKeyBytes is the minimum acceptable length of JWT_KEY. HS256 requires
	// keys at least as long as the hash output (32 bytes / 256 bits).
	minJWTKeyBytes = 32
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

	jwtKey := os.Getenv("JWT_KEY")
	if len(jwtKey) < minJWTKeyBytes {
		logger.ErrorContext(ctx, "JWT_KEY environment variable is too short", slog.Int("min_bytes", minJWTKeyBytes))
		return
	}

	database := postgres.New(pool)
	identityRepository := storage.NewIdentityRepository(database, time.Now)

	server.Register(
		iam.Endpoints(logger, uuidV4Generator{}, identityRepository, jwtKey, bcrypt.DefaultCost, time.Now)...,
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

func (a *pgxSlogAdapter) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
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

	a.logger.LogAttrs(ctx, lvl, msg, attrs...) //nolint:sloglint // Forwarding pgx tracelog messages; the dynamic msg is intentional for this adapter.
}
