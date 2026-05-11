package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver used by goose
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	pgmigrations "github.com/nickbryan/objectory/api/internal/storage/postgres/migrations"
)

const (
	templateDBName = "template_iam"
	containerLabel = "objectory-test-postgres"

	// dbNameRandomBytes is the number of random bytes used to suffix each
	// per-test database name. 8 bytes → 16 hex chars, ~10^19 collision space.
	dbNameRandomBytes = 8
	// containerReadyOccurrences is the number of "ready to accept connections"
	// log lines required before treating the container as up. The image emits
	// the line twice (init / final startup); waiting for both avoids a window
	// where the server bounces during initdb.
	containerReadyOccurrences = 2
	containerStartupTimeout   = 60 * time.Second
	// templateMigrationLockID is the advisory-lock key used to serialize
	// goose migrations on the template DB across concurrent test binaries
	// (sync.Once only synchronizes within a single process).
	templateMigrationLockID int64 = 0x6F626A_74656D70 // "obj_temp"
)

// Package-level state intentionally guards the one-time-per-process container
// and template setup behind sync.OnceValue. Tests share the container and
// template across goroutines; the OnceValue holds the setup outcome so every
// test in the process sees the same result.
//
//nolint:gochecknoglobals // sync.Once handshake state and shared resource locator
var (
	setupOnce = sync.OnceValue(func() error { return setupTemplate(context.Background()) })
	baseDSN   string // "postgres://user:pass@host:port" — populated by setupTemplate
)

// NewTestDB returns a *pgxpool.Pool connected to a freshly-cloned test
// database. The database is created from a once-migrated template_iam
// database via CREATE DATABASE ... TEMPLATE ..., which Postgres makes cheap.
// On test cleanup the pool is closed and the database is dropped.
func NewTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if err := setupOnce(); err != nil {
		t.Fatalf("postgres test setup: %v", err)
	}

	dbName := "test_" + randHex(dbNameRandomBytes)

	ctx := context.Background()

	adminPool, err := pgxpool.New(ctx, baseDSN+"/postgres")
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	defer adminPool.Close()

	createSQL := fmt.Sprintf(`CREATE DATABASE %q TEMPLATE %q`, dbName, templateDBName)
	if _, err := adminPool.Exec(ctx, createSQL); err != nil {
		t.Fatalf("create test db %q: %v", dbName, err)
	}

	pool, err := pgxpool.New(ctx, baseDSN+"/"+dbName)
	if err != nil {
		t.Fatalf("connect test pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		drop, dErr := pgxpool.New(context.Background(), baseDSN+"/postgres")
		if dErr != nil {
			t.Logf("cleanup: connect admin pool: %v", dErr)
			return
		}
		defer drop.Close()

		dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, dbName)
		if _, err := drop.Exec(context.Background(), dropSQL); err != nil {
			t.Logf("cleanup: drop db %q: %v", dbName, err)
		}
	})

	return pool
}

func setupTemplate(ctx context.Context) error {
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithReuseByName(containerLabel),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(containerReadyOccurrences).
				WithStartupTimeout(containerStartupTimeout),
		),
	)
	if err != nil {
		return fmt.Errorf("start postgres container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return fmt.Errorf("container port: %w", err)
	}

	baseDSN = "postgres://test:test@" + net.JoinHostPort(host, port.Port())

	if err := ensureTemplate(ctx); err != nil {
		return fmt.Errorf("ensure template: %w", err)
	}

	return nil
}

func ensureTemplate(ctx context.Context) error {
	// Open the lock connection on the admin "postgres" DB via database/sql so
	// the advisory lock is held on a single pinned session. Holding it on a
	// pgxpool connection would risk the pool reassigning the conn; holding it
	// on template_iam would block CREATE DATABASE ... TEMPLATE in peer
	// processes (Postgres rejects TEMPLATE clones while the source DB has
	// open connections).
	lockConn, err := sql.Open("pgx", baseDSN+"/postgres")
	if err != nil {
		return fmt.Errorf("open admin lock conn: %w", err)
	}
	defer func() { _ = lockConn.Close() }()

	lockConn.SetMaxOpenConns(1)

	if _, err := lockConn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", templateMigrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = lockConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", templateMigrationLockID)
	}()

	adminPool, err := pgxpool.New(ctx, baseDSN+"/postgres")
	if err != nil {
		return fmt.Errorf("connect admin pool: %w", err)
	}
	defer adminPool.Close()

	var exists bool
	if err := adminPool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", templateDBName).Scan(&exists); err != nil {
		return fmt.Errorf("check template existence: %w", err)
	}

	if !exists {
		if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, templateDBName)); err != nil {
			return fmt.Errorf("create template db: %w", err)
		}
	}

	db, err := sql.Open("pgx", baseDSN+"/"+templateDBName)
	if err != nil {
		return fmt.Errorf("open template db: %w", err)
	}
	defer func() { _ = db.Close() }()

	goose.SetBaseFS(pgmigrations.FS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("testutil: rand.Read: " + err.Error())
	}

	return hex.EncodeToString(b)
}
