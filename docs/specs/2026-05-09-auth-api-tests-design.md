# Design: Auth API test suite

**Date:** 2026-05-09
**Scope:** API side only (`api/internal/iam`, `api/internal/storage`, plus extracted helpers).
**Status:** Approved, pending user review.

## Goals

- Cover the auth API (identity creation, token issuance, JWT guard, identity lookup, repository persistence) with unit and integration tests sufficient to land at 90%+ statement coverage on the in-scope packages.
- Establish reusable testing conventions — fakes, fixtures, response assertions, DB lifecycle — that subsequent packages adopt without re-litigating decisions.
- Run the entire suite with `t.Parallel()` by default; failures must not depend on test ordering.
- Test only the public API of each package (external `_test` packages, no white-box tests).

## Non-goals

- Web-side tests (`web/internal/...`). Out of scope; will be a separate spec.
- Browser/E2E tests against the running stack.
- Coverage gates in CI. Target is 90%+ but enforcement deferred until numbers stabilise.
- Performance / load tests.

## Test taxonomy

Three test homes, each with a single clear purpose.

### 1. Unit tests — `api/internal/iam/*_test.go`

- Package `iam_test`.
- No build tag; run by plain `go test ./...`.
- No DB, no docker. Hand-rolled fakes from `api/internal/testutil` for `IdentityRepository` and `UUIDV4Generator`. Fixed clock via injected `now func() time.Time`.
- Handlers exercised through `httputil.NewServer(...)` + `server.ServeHTTP(rec, req)` against an `httptest.ResponseRecorder`.
- Cover: `tokenCreateHandler`, `identityCreateHandler`, `identityMeHandler`, `NewJWTGuard`, `CurrentIdentityFromContext`, `NewPasswordHash` / `Matches` / `NewHashedPassword`.

### 2. Repository integration tests — `api/internal/storage/*_integration_test.go`

- Package `storage_test`.
- Build tag `//go:build integration`.
- Run against a real Postgres testcontainer; each test gets its own database cloned from `template_iam`.
- Cover: every public method on `IdentityRepository` (`Create`, `Find`, `FindByEmail`) including error mappings (`pgerrcode.UniqueViolation` → `iam.ErrDuplicateIdentity`, `pgx.ErrNoRows` → `iam.ErrIdentityNotFound`) and timestamp behaviour (asserting `created_at` / `updated_at` against the injected fixed clock).

### 3. Feature tests — `api/tests/*_test.go`

- Package `tests`.
- Build tag `//go:build integration`.
- Wire up the real `iam.Endpoints` with the real `storage.IdentityRepository` against a real Postgres. Test as HTTP: send request → assert status, headers, response body via `cmp.Diff` → query DB to verify side effects (rows created, password hash matches, timestamps match the injected clock).
- Located outside `api/internal/` so the package can only import via the public API — naturally enforces the no-white-box-tests rule.

### Conventions across all three layers

- **Test cases** are `map[string]struct{...}` literals with a single nested `t.Run(name, ...)`. Each case calls `t.Parallel()`. Where table-driven hurts readability (e.g. multi-step flows), nest `t.Run` calls without a table.
- **Comparisons** use `github.com/google/go-cmp/cmp`. No mocking frameworks (no `testify/mock`, no `gomock`).
- **Response assertions** use `testutil.JSONResponse` / `testutil.ProblemResponse`, which take JSON string literals and decode both sides for `cmp.Diff` — the test source reads like the wire contract.
- **Logger** in tests is `slog.New(slog.NewJSONHandler(io.Discard, nil))` unless a test explicitly inspects log output.

## `api/internal/testutil` package

Flat layout, four files. Package name: `testutil`.

### `testutil/postgresdb.go`

- `NewTestDB(t *testing.T) *pgxpool.Pool` — primary entry point for any DB-touching test.
  - Internally guarded by `sync.Once`:
    - `testcontainers-go` Postgres container with `Reuse: true` (boots fresh or attaches to an existing reused container).
    - `CREATE DATABASE template_iam IF NOT EXISTS` (idempotent).
    - Apply embedded goose migrations to `template_iam` (idempotent — goose handles its own locking around `goose_db_version`).
  - Per call: `CREATE DATABASE test_<random> TEMPLATE template_iam`, connect via `pgxpool.Pool`, register `t.Cleanup` to close the pool and `DROP DATABASE test_<random>`.
- Random suffix ensures cross-process isolation when multiple test binaries run concurrently.
- No `TestMain` boilerplate required in caller packages.

### `testutil/iamfake.go`

- `IdentityRepository` — hybrid fake. In-memory `map[uuid.UUID]iam.Identity` with realistic semantics: duplicate email → `iam.ErrDuplicateIdentity`; missing → `iam.ErrIdentityNotFound`. Concurrency-safe via internal `sync.Mutex`.
  - Public fields `CreateErr`, `FindErr`, `FindByEmailErr` for error injection. When set, the next call to that method returns the error before touching the map.
- `UUIDV4Generator` — pre-seeded with `[]uuid.UUID`; cycles through them. `Err` field for error injection.

### `testutil/httpassert.go`

- `JSONResponse(t, rec *httptest.ResponseRecorder, wantStatus int, wantJSON string)`:
  - Asserts `rec.Code == wantStatus`.
  - Asserts `Content-Type` starts with `application/json`.
  - `json.Unmarshal` both `rec.Body.Bytes()` and `wantJSON` into `any`, runs `cmp.Diff(want, got)`. Decode normalizes whitespace and key order; `cmp.Diff` produces structured failure output.
- `ProblemResponse(t, rec, wantStatus int, wantProblemJSON string)`:
  - Same approach, specialised for RFC 9457 problem responses emitted by `httputil/problem`.

### `testutil/fixtures.go`

- `JWTKey` — 32-byte canned key (≥ HS256 minimum).
- `KnownIdentity()` — `iam.Identity` with stable UUID, name, email, and a precomputed bcrypt hash at cost 4 (computed once at package `init` time). Returned via `iam.NewHashedPassword(hash)` to skip cost.
- `KnownPassword` — plaintext that matches the `KnownIdentity` hash.
- `FixedTime` — `time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)`, the fixed instant for clock fakes.
- `Clock()` — returns `func() time.Time { return FixedTime }`.

## Production code refactors

All "make a knob configurable at the seam" changes. None alters runtime behaviour in production.

### 1. Clock injection through `iam.Endpoints`

- Add `now func() time.Time` parameter to `iam.Endpoints`.
- `tokenCreateHandler` closure captures it; replace the two `time.Now()` calls in `api/internal/iam/token.go:54-56` with `now()`.
- `main.go` passes `time.Now`. Tests pass `testutil.Clock()`.

### 2. UUID generator into `tokenCreateHandler`

- `tokenCreateHandler(logger, identities, jwtKey)` becomes `tokenCreateHandler(logger, identities, uuidGenerator, jwtKey, now)`.
- `iam.Endpoints` already takes `UUIDV4Generator`; pass the same instance into the token handler.
- Replace `uuid.NewRandom()` in `api/internal/iam/token.go:43` with `uuidGenerator.GenerateUUIDV4()`. Wrap the `[16]byte` return into `uuid.UUID` via `uuid.UUID(bytes)`.

### 3. Password cost into `iam.Endpoints`

- Add `passwordCost int` parameter.
- Plumb into `identityCreateHandler` closure; passed to `NewPasswordHash`.
- `main.go` passes `bcrypt.DefaultCost`. Tests pass `bcrypt.MinCost`.
- Validation is the caller's responsibility, matching the existing pattern for `jwtKey` (validated in `main.go`, not in `Endpoints`). Since `main.go` passes a named constant, the only realistic "bad cost" path is a buggy test — and `bcrypt.GenerateFromPassword` already errors on costs outside its allowed range, so a misconfigured test fails noisily without extra validation logic.

### 4. `NewPasswordHash` signature change

- `NewPasswordHash(password string, cost int) (PasswordHash, error)`.
- Single production caller (`identityCreateHandler`) updates to pass the cost from its closure.

### 5. Extract `pgxSlogAdapter` to `api/internal/log/pgxlog`

- New package `pgxlog`. Public surface: `pgxlog.NewAdapter(logger *slog.Logger) *Adapter`, where `*Adapter` satisfies `tracelog.Logger`.
- `main.go` imports it: `Tracer: &tracelog.TraceLog{Logger: pgxlog.NewAdapter(logger), LogLevel: tracelog.LogLevelInfo}`.
- The level-translation switch becomes table-testable from `pgxlog_test`.

### 6. Extract `uuidV4Generator{}` to `api/internal/uuidgen`

- New package `uuidgen`. Public surface: `uuidgen.New() iam.UUIDV4Generator`.
- `main.go` imports it: `iam.Endpoints(..., uuidgen.New(), ...)`.
- `uuidgen_test` confirms `GenerateUUIDV4` returns a valid v4 UUID.

### 7. Embedded migrations entry point co-located with SQL files

- Go's `//go:embed` directive forbids `..` in paths, so `testutil` cannot embed `../storage/postgres/migrations/*.sql` directly.
- Add a new file `api/internal/storage/postgres/migrations/migrations.go`:
  ```go
  package migrations

  import "embed"

  //go:embed *.sql
  var FS embed.FS
  ```
- `testutil/postgresdb.go` imports the package and reads `migrations.FS` to drive goose. Production code may use the same FS later if it ever needs in-process migrations on startup.

### Resulting `iam.Endpoints` signature

```go
func Endpoints(
    logger *slog.Logger,
    uuidV4Generator UUIDV4Generator,
    identityRepository IdentityRepository,
    jwtKey string,
    passwordCost int,
    now func() time.Time,
) httputil.EndpointGroup
```

Six parameters; at the edge of comfortable. If a future addition pushes to seven, collapse into a `Config` struct.

## Test infrastructure and DB lifecycle

```
testutil.NewTestDB(t)
  ├─ once.Do(setup)
  │    ├─ testcontainers.GenericContainer(Reuse: true)
  │    │    → finds existing Postgres container or starts a new one
  │    ├─ CREATE DATABASE template_iam IF NOT EXISTS  (idempotent)
  │    ├─ goose.Up(templateConn, embeddedMigrations)  (idempotent; goose handles concurrent migration safety)
  │    └─ mark template ready
  ├─ CREATE DATABASE test_<random> TEMPLATE template_iam
  ├─ connect via pgxpool
  ├─ register t.Cleanup(close pool, DROP DATABASE test_<random>)
  └─ return *pgxpool.Pool
```

**Within a single process:** `sync.Once` ensures one-time setup; per-test DBs are independent; `t.Parallel` is safe.

**Across test processes** (`go test -p N` runs packages concurrently): the testcontainer is shared via `Reuse: true`. `CREATE DATABASE template_iam` and `goose.Up` are idempotent; goose's internal locking handles the migration-race case. Per-test DB names use a random suffix so cross-process collision risk is negligible.

**Container reuse beyond a single invocation:** testcontainers leaves the container running after the test process exits. Subsequent `go test -tags=integration ./...` invocations attach via `Reuse: true` and skip boot — locally, ~3s saved per cold run, instant on warm re-runs. The `ryuk` reaper container that `testcontainers-go` spins up cleans the testcontainer after a configurable idle window.

**Migrations as embedded fixtures:** the SQL files at `api/internal/storage/postgres/migrations/` get an embed entry point in the same directory (production-code refactor item 7), exposing `migrations.FS`. `testutil/postgresdb.go` imports that package and feeds the `embed.FS` to goose. Tests use the same migration files production runs — no parallel definitions to drift, no `..` workaround.

**Environment compatibility:** CI runners without privileged Docker sockets need `TESTCONTAINERS_RYUK_DISABLED=true`. To be set as a job-level env var when wiring `.github/workflows/`.

## Coverage scope

- **In scope** (90%+ target): `api/internal/iam`, `api/internal/storage`, `api/internal/log/pgxlog`, `api/internal/uuidgen`.
- **Excluded:**
  - `api/internal/storage/postgres` — sqlc-generated; tested transitively via `storage` integration tests.
  - `api/internal/testutil` — test code itself.
  - `api/main.go` — wiring (`os.Getenv`, pool construction, `server.Serve`); no business logic remains after extracting `pgxSlogAdapter` and `uuidV4Generator`.

## Make and CI integration

```make
.PHONY: test test-integration test-cover

test:
	go test -race -shuffle=on ./...

test-integration:
	go test -race -shuffle=on -tags=integration ./...

test-cover:
	go test -race -shuffle=on -tags=integration \
	    -coverprofile=coverage.out \
	    -coverpkg=./api/internal/iam,./api/internal/storage,./api/internal/log/pgxlog,./api/internal/uuidgen \
	    ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
```

`-coverpkg` is explicit so feature tests in `api/tests/` count toward each in-scope package's coverage. `.gitignore` adds `coverage.out` and `coverage.html`.

CI workflow setup is a separate TODO item; this design assumes the existing `## CI` entry in `TODO.md` will be picked up after the test suite lands. CI will run `make test-integration` with `TESTCONTAINERS_RYUK_DISABLED=true`.

## Packages to add

- `github.com/google/go-cmp/cmp` (and `cmp/cmpopts` if needed for time tolerance, though no current case requires it).
- `github.com/testcontainers/testcontainers-go`.
- `github.com/testcontainers/testcontainers-go/modules/postgres`.
- (Existing) `github.com/jackc/pgx/v5/pgxpool`, `github.com/google/uuid`, `golang.org/x/crypto/bcrypt`, `github.com/golang-jwt/jwt/v5` — already in use.
- (Existing) goose — already in use for production migrations.

## ADRs to write alongside the implementation

- **ADR-0002**: Test taxonomy and build-tagged integration tests. Records the three test homes (unit co-located, repo integration tagged, feature tests in `api/tests/`), the build-tag mechanism, and the rationale for placing feature tests outside `api/internal/`. Lands when the first integration test file lands.
- **ADR-0003**: Postgres test isolation via per-test database cloned from a migrated template. Records the testcontainers + `Reuse: true` + `sync.Once` lifecycle, why per-test database (vs. transaction rollback, schema, truncate), and the implications for parallel testing. Lands with `testutil/postgresdb.go`.
- **ADR-0004**: Testing conventions. Records: hand-rolled fakes over mocking frameworks (with the in-memory + error-injection hybrid), `map[string]struct{...}` table-driven cases with a single `t.Run`, JSON literals + `cmp.Diff` for response assertions, public-API-only test packages, fixed clock + UUID injection over fuzzy assertions. Lands when `testutil/iamfake.go` and `testutil/httpassert.go` land.
- **ADR-0005**: Configurable seams in `iam.Endpoints`. Records the convention of injecting clock, UUID generator, and bcrypt cost as public-API parameters rather than relying on global state or test-only hooks. Lands with the `iam.Endpoints` refactor.

## Risks and open items

- **Bcrypt cost regression in production.** Mitigated by `main.go` passing the named constant `bcrypt.DefaultCost` directly — code review catches a change to that line. `bcrypt.GenerateFromPassword` itself errors on out-of-range costs, so even an accidental low value fails fast at the first registration attempt.
- **Migration drift between test and production.** Mitigated by `//go:embed` of the canonical migration files — any new migration file is automatically picked up by tests.
- **`testcontainers-go` and Docker socket assumptions.** Specific CI runners may need `TESTCONTAINERS_RYUK_DISABLED=true`. Documented in this spec; will be set in the CI workflow file when added.
- **Six-parameter `iam.Endpoints` signature.** At the edge of comfortable. Future additions trigger a `Config`-struct collapse.
