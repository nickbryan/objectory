// Package main is the entrypoint for the objectory API server.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/pgxlog"
	"github.com/nickbryan/objectory/api/internal/storage"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
	"github.com/nickbryan/objectory/api/internal/uuidgen"
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
		Logger:   pgxlog.NewAdapter(logger),
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
		iam.Endpoints(logger, uuidgen.New(), identityRepository, jwtKey, bcrypt.DefaultCost, time.Now)...,
	)

	server.Serve(ctx)
}
