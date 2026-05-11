# ADR-0003: Postgres test isolation via per-test database from a migrated template

## Date

2026-05-09

## Context

Integration and feature tests need a real Postgres database. Tests must run with `t.Parallel()` everywhere, so each test needs its own isolated DB state. Options considered: per-test transaction rollback, per-test schema with shared search_path, truncate-between-tests on a single DB, per-test database cloned from a template.

## Decision

Use **per-test database cloned from a once-migrated `template_iam`**:

- A single Postgres testcontainer boots once, shared across test processes via `testcontainers.WithReuseByName`.
- A `sync.OnceValue` lazy init creates `template_iam` and applies goose migrations to it. Concurrent test binaries are serialized via a Postgres session-scoped advisory lock (`pg_advisory_lock`) held during `goose.UpContext` — `sync.OnceValue` only synchronizes within a process, so a cross-process lock is needed to prevent `goose_db_version` creation races.
- `testutil.NewTestDB(t)` runs `CREATE DATABASE test_<random> TEMPLATE template_iam`, returns a connected `*pgxpool.Pool`, and registers `t.Cleanup` to close the pool and `DROP DATABASE`.

Migrations are embedded in the migrations package itself (`api/internal/storage/postgres/migrations/migrations.go`) and exposed as `migrations.FS`, used by both the test runner and (potentially) production startup.

## Rationale

- **Full parallel isolation** — each test gets a fresh, independent database. No mutex over a shared DB; no transaction scoping that would conflict with code that manages its own transactions.
- **Fast** — Postgres clones from a template in tens of milliseconds. Migrations run once per process, not per test.
- **Realistic** — tests exercise the same migrations production runs, with no parallel SQL definitions to drift.
- **Container reuse** — `WithReuseByName` keeps the testcontainer warm across `go test` invocations, saving ~3s per cold dev-loop run.
- **Cross-process safety** — the advisory lock means even `go test -tags=integration ./...` (which spawns one binary per package in parallel) deterministically migrates the template exactly once.

## Consequences

- Tests require Docker daemon access. CI runners without privileged sockets need `TESTCONTAINERS_RYUK_DISABLED=true`.
- The `migrations` Go package becomes a public-ish dependency for the test runner — production may also adopt it for in-process startup migrations later.
- Per-test DB names use a random hex suffix; cross-process collision risk is negligible.
- If a test leaks (panic mid-cleanup), an orphan `test_<random>` database remains. The reaper container cleans it up after container idle timeout, or it survives until next manual cleanup. Acceptable.
