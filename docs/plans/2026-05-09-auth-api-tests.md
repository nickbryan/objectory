# Auth API Test Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cover the auth API (`api/internal/iam` and `api/internal/storage`) with unit, repository-integration, and feature tests sufficient to land at 90%+ coverage on in-scope packages, while establishing reusable testing conventions for the rest of the project.

**Architecture:** Three test homes — unit tests co-located in `api/internal/iam`, repository integration tests co-located in `api/internal/storage` behind `//go:build integration`, feature tests in `api/tests/` behind the same tag. Postgres testcontainer with a once-migrated `template_iam` database; each test clones a fresh database from it for full parallel isolation. Hand-rolled fakes (in-memory + error injection) live in `api/internal/testutil`. Five small production-code refactors make `iam.Endpoints` fully deterministic by injecting clock, UUID generator, and bcrypt cost.

**Tech Stack:** Go 1.26, `pgx/v5`, `pgxpool`, `goose v3` (existing), `httputil` (existing), `golang-jwt/jwt/v5` (existing), `bcrypt` (existing). New direct deps: `github.com/google/go-cmp`, `github.com/testcontainers/testcontainers-go`, `github.com/testcontainers/testcontainers-go/modules/postgres`.

**Reference spec:** `docs/specs/2026-05-09-auth-api-tests-design.md`

---

## File Structure

### Files to create

| Path | Responsibility |
|---|---|
| `api/internal/storage/postgres/migrations/migrations.go` | Embed entry point exposing `migrations.FS` for goose |
| `api/internal/log/pgxlog/pgxlog.go` | Extracted `pgxSlogAdapter` from `main.go` |
| `api/internal/log/pgxlog/pgxlog_test.go` | Tests for the level-translation switch |
| `api/internal/uuidgen/uuidgen.go` | Extracted `uuidV4Generator` from `main.go` |
| `api/internal/uuidgen/uuidgen_test.go` | Tests for the v4 generator |
| `api/internal/testutil/fixtures.go` | Canned JWT key, fixed time, known identity, known password, clock helper |
| `api/internal/testutil/iamfake.go` | Hybrid in-memory + error-injection fakes for `iam.IdentityRepository` and `iam.UUIDV4Generator` |
| `api/internal/testutil/httpassert.go` | `JSONResponse` and `ProblemResponse` helpers |
| `api/internal/testutil/postgresdb.go` | Testcontainer lifecycle, template migration, per-test DB clone |
| `api/internal/iam/identity_test.go` | Unit tests for `NewPasswordHash`/`Matches`, `identityCreateHandler`, `identityMeHandler` |
| `api/internal/iam/token_test.go` | Unit tests for `tokenCreateHandler` |
| `api/internal/iam/auth_test.go` | Unit tests for `NewJWTGuard` and `CurrentIdentityFromContext` |
| `api/internal/storage/identity_integration_test.go` | Repo integration tests for `IdentityRepository.Create/Find/FindByEmail` |
| `api/tests/identities_test.go` | Feature tests for `/iam/identities` endpoints |
| `api/tests/tokens_test.go` | Feature tests for `/iam/tokens` and `/iam/identities/me` |
| `docs/adr/0002-test-taxonomy.md` | ADR: three test homes + build tags |
| `docs/adr/0003-postgres-test-isolation.md` | ADR: per-test database from migrated template |
| `docs/adr/0004-testing-conventions.md` | ADR: fakes, table-driven map cases, JSON-literal assertions |
| `docs/adr/0005-iam-configurable-seams.md` | ADR: clock/UUID/cost injection in `iam.Endpoints` |

### Files to modify

| Path | Change |
|---|---|
| `api/internal/iam/identity.go` | `NewPasswordHash` takes `cost int` |
| `api/internal/iam/token.go` | `tokenCreateHandler` takes `UUIDV4Generator` and `now func() time.Time` |
| `api/internal/iam/endpoints.go` | `Endpoints` takes `passwordCost int` and `now func() time.Time`; passes UUID generator into both handlers |
| `api/main.go` | Wire new params; remove inlined `pgxSlogAdapter` and `uuidV4Generator` |
| `Makefile` | Replace `test` target; add `test-integration`, `test-cover` |
| `.gitignore` | Add `coverage.out`, `coverage.html` |
| `go.mod` / `go.sum` | Promote `go-cmp` to direct dep; add `testcontainers-go` |

---

## Task 1: Add test dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [x] **Step 1: Add testcontainers-go and promote go-cmp**

Run:

```bash
cd /Users/nick/code/objectory
go get github.com/google/go-cmp@latest
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
go mod tidy
```

Expected: `go.mod` shows `github.com/google/go-cmp` and the two `testcontainers-go` packages as direct dependencies. `go.sum` updates accordingly.

- [x] **Step 2: Verify build still works**

Run: `go build ./...`
Expected: clean build, no output.

- [x] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add go-cmp and testcontainers-go for the test suite"
```

---

## Task 2: Migrations embed entry point

**Files:**
- Create: `api/internal/storage/postgres/migrations/migrations.go`

- [x] **Step 1: Create the embed file**

Write `api/internal/storage/postgres/migrations/migrations.go`:

```go
// Package migrations exposes the SQL migration files as an embed.FS so that
// goose-based runners (production startup, tests) can apply them without
// shelling out to the goose CLI.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [x] **Step 2: Verify build**

Run: `go build ./api/internal/storage/postgres/migrations/...`
Expected: clean build.

- [x] **Step 3: Commit**

```bash
git add api/internal/storage/postgres/migrations/migrations.go
git commit -m "refactor(api/migrations): expose migration files as embed.FS"
```

---

## Task 3: testutil/fixtures.go

**Files:**
- Create: `api/internal/testutil/fixtures.go`

- [x] **Step 1: Write the file**

Write `api/internal/testutil/fixtures.go`:

```go
// Package testutil provides shared helpers for the api module's test suites:
// fakes for iam collaborators, response assertions, fixtures, and Postgres
// container/database lifecycle.
package testutil

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/objectory/api/internal/iam"
)

// JWTKey is a fixed 32-byte signing key shared by every test that touches
// JWTs. HS256 requires keys of at least 32 bytes (256 bits).
const JWTKey = "test-jwt-key-32-bytes-padding!!!"

// KnownPassword is the plaintext that matches the hash in KnownIdentity.
const KnownPassword = "correct-horse-battery-staple"

// FixedTime is the canonical instant used by tests that inject a clock.
var FixedTime = time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

// Clock returns a now() function that always returns FixedTime, suitable for
// passing to iam.Endpoints or storage.NewIdentityRepository in tests.
func Clock() func() time.Time {
	return func() time.Time { return FixedTime }
}

// KnownIdentityID is the stable UUID used for the fixture identity.
var KnownIdentityID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

var knownIdentityHash []byte

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte(KnownPassword), bcrypt.MinCost)
	if err != nil {
		panic("testutil: precomputing fixture password hash: " + err.Error())
	}
	knownIdentityHash = hash
}

// KnownIdentity returns a fixture iam.Identity with a stable ID, name, email
// and a precomputed bcrypt hash (cost = bcrypt.MinCost) for the password
// KnownPassword.
func KnownIdentity() iam.Identity {
	return iam.Identity{
		ID:       KnownIdentityID,
		Name:     "Known Test User",
		Email:    "known@example.com",
		Password: iam.NewHashedPassword(knownIdentityHash),
	}
}
```

- [x] **Step 2: Verify build**

Run: `go build ./api/internal/testutil/...`
Expected: clean build.

- [x] **Step 3: Commit**

```bash
git add api/internal/testutil/fixtures.go
git commit -m "test(testutil): add fixtures for JWT key, clock, and known identity"
```

---

## Task 4: testutil/iamfake.go

**Files:**
- Create: `api/internal/testutil/iamfake.go`

- [x] **Step 1: Write the file**

Write `api/internal/testutil/iamfake.go`:

```go
package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/iam"
)

// IdentityRepository is an in-memory fake of iam.IdentityRepository with
// optional error injection. Default behaviour mirrors the real repository:
// duplicate emails return iam.ErrDuplicateIdentity, missing rows return
// iam.ErrIdentityNotFound. Methods are safe for concurrent use.
type IdentityRepository struct {
	mu      sync.Mutex
	byID    map[uuid.UUID]iam.Identity
	byEmail map[string]uuid.UUID

	// CreateErr, FindErr, FindByEmailErr force the next call to that method
	// to return the given error before touching the in-memory store.
	CreateErr      error
	FindErr        error
	FindByEmailErr error
}

// NewIdentityRepository returns an empty fake.
func NewIdentityRepository() *IdentityRepository {
	return &IdentityRepository{
		byID:    make(map[uuid.UUID]iam.Identity),
		byEmail: make(map[string]uuid.UUID),
	}
}

// Seed inserts an identity directly, bypassing duplicate checks. Use this
// from tests to set up preconditions.
func (r *IdentityRepository) Seed(identity iam.Identity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[identity.ID] = identity
	r.byEmail[identity.Email] = identity.ID
}

func (r *IdentityRepository) Create(_ context.Context, identity iam.Identity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CreateErr != nil {
		return r.CreateErr
	}
	if _, exists := r.byEmail[identity.Email]; exists {
		return iam.ErrDuplicateIdentity
	}
	r.byID[identity.ID] = identity
	r.byEmail[identity.Email] = identity.ID
	return nil
}

func (r *IdentityRepository) Find(_ context.Context, id uuid.UUID) (*iam.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FindErr != nil {
		return nil, r.FindErr
	}
	identity, ok := r.byID[id]
	if !ok {
		return nil, iam.ErrIdentityNotFound
	}
	return &identity, nil
}

func (r *IdentityRepository) FindByEmail(_ context.Context, email string) (*iam.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FindByEmailErr != nil {
		return nil, r.FindByEmailErr
	}
	id, ok := r.byEmail[email]
	if !ok {
		return nil, iam.ErrIdentityNotFound
	}
	identity := r.byID[id]
	return &identity, nil
}

// UUIDV4Generator is a fake of iam.UUIDV4Generator that returns pre-seeded
// UUIDs in order. It supports error injection via Err.
type UUIDV4Generator struct {
	mu    sync.Mutex
	UUIDs []uuid.UUID
	idx   int

	// Err forces the next call to return this error.
	Err error
}

// NewUUIDV4Generator returns a generator pre-seeded with the given UUIDs.
func NewUUIDV4Generator(uuids ...uuid.UUID) *UUIDV4Generator {
	return &UUIDV4Generator{UUIDs: uuids}
}

func (g *UUIDV4Generator) GenerateUUIDV4() ([16]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Err != nil {
		return [16]byte{}, g.Err
	}
	if g.idx >= len(g.UUIDs) {
		return [16]byte{}, errors.New("testutil.UUIDV4Generator: ran out of pre-seeded UUIDs")
	}
	next := g.UUIDs[g.idx]
	g.idx++
	return [16]byte(next), nil
}
```

- [x] **Step 2: Verify build**

Run: `go build ./api/internal/testutil/...`
Expected: clean build.

- [x] **Step 3: Commit**

```bash
git add api/internal/testutil/iamfake.go
git commit -m "test(testutil): add hybrid in-memory + error-injection iam fakes"
```

---

## Task 5: testutil/httpassert.go

**Files:**
- Create: `api/internal/testutil/httpassert.go`

- [x] **Step 1: Write the file**

Write `api/internal/testutil/httpassert.go`:

```go
package testutil

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// JSONResponse asserts that the recorder captured the expected status,
// application/json content-type, and JSON body. Both the actual body and
// wantJSON are decoded into any before being diffed, so key ordering and
// whitespace are normalised.
func JSONResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantJSON string) {
	t.Helper()
	assertJSONLike(t, rec, wantStatus, wantJSON, "application/json")
}

// ProblemResponse is the same as JSONResponse but expects an
// application/problem+json content-type, matching the RFC 9457 responses
// emitted by httputil/problem.
func ProblemResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantJSON string) {
	t.Helper()
	assertJSONLike(t, rec, wantStatus, wantJSON, "application/problem+json")
}

func assertJSONLike(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantJSON, wantContentTypePrefix string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Errorf("status: got %d, want %d\nbody: %s", rec.Code, wantStatus, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, wantContentTypePrefix) {
		t.Errorf("content-type: got %q, want prefix %q", ct, wantContentTypePrefix)
	}

	var got, want any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response body: %v\nbody: %s", err, rec.Body.String())
	}
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("decode want JSON: %v\njson: %s", err, wantJSON)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("body mismatch (-want +got):\n%s", diff)
	}
}
```

- [x] **Step 2: Verify build**

Run: `go build ./api/internal/testutil/...`
Expected: clean build.

- [x] **Step 3: Commit**

```bash
git add api/internal/testutil/httpassert.go
git commit -m "test(testutil): add JSONResponse and ProblemResponse helpers"
```

---

## Task 6: Refactor NewPasswordHash and identityCreateHandler — drive with a test

This task does TDD-style: write a unit test for `identityCreateHandler` against the new `iam.Endpoints` signature, watch it fail (signature mismatch), then refactor `NewPasswordHash`, `iam.Endpoints`, and `main.go` to make it pass.

**Files:**
- Create: `api/internal/iam/identity_test.go`
- Modify: `api/internal/iam/identity.go`
- Modify: `api/internal/iam/endpoints.go`
- Modify: `api/internal/iam/token.go` (signature update only — UUID generator wiring lands in Task 7)
- Modify: `api/main.go`

- [x] **Step 1: Write the failing test**

Write `api/internal/iam/identity_test.go`:

```go
package iam_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func newServer(t *testing.T, repo iam.IdentityRepository, gen iam.UUIDV4Generator) *httputil.Server {
	t.Helper()
	server := httputil.NewServer(quietLogger())
	server.Register(iam.Endpoints(
		quietLogger(),
		gen,
		repo,
		testutil.JWTKey,
		bcrypt.MinCost,
		testutil.Clock(),
	)...)
	return server
}

func TestIdentityCreateHandler_Success(t *testing.T) {
	t.Parallel()

	repo := testutil.NewIdentityRepository()
	newID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	gen := testutil.NewUUIDV4Generator(newID)

	server := newServer(t, repo, gen)

	body := bytes.NewBufferString(`{
		"name": "Alice",
		"email": "alice@example.com",
		"password": "supersecret",
		"passwordConfirmation": "supersecret"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/iam/identities", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	testutil.JSONResponse(t, rec, http.StatusCreated, `{
		"data": {"id": "11111111-1111-1111-1111-111111111111"}
	}`)

	if got, err := repo.Find(req.Context(), newID); err != nil {
		t.Fatalf("identity not stored: %v", err)
	} else if got.Email != "alice@example.com" || got.Name != "Alice" {
		t.Errorf("stored identity mismatch: got %+v", got)
	}
}
```

- [x] **Step 2: Run the test, expect a compile failure**

Run: `go test ./api/internal/iam/...`
Expected: build error referencing `iam.Endpoints` (currently 4 params, test passes 6).

- [x] **Step 3: Update `NewPasswordHash` and `identityCreateHandler` together**

Both edits happen in `api/internal/iam/identity.go` and must land in the same change so the file compiles.

Replace `NewPasswordHash`:

```go
// NewPasswordHash creates a new PasswordHash from a password string at the
// given bcrypt cost. cost should be bcrypt.DefaultCost in production;
// bcrypt.MinCost in tests.
func NewPasswordHash(password string, cost int) (PasswordHash, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return PasswordHash{}, fmt.Errorf("generating hash from password: %w", err)
	}

	return PasswordHash{hash: hash}, nil
}
```

Replace `identityCreateHandler` to take `passwordCost`:

```go
func identityCreateHandler(logger *slog.Logger, uuidGenerator UUIDV4Generator, identities IdentityRepository, passwordCost int) http.Handler {
	type (
		request struct {
			Name                 string `json:"name"                 validate:"required,max=64"`
			Email                string `json:"email"                validate:"required,email"`
			Password             string `json:"password"             validate:"required,min=8,max=64,eqfield=PasswordConfirmation"`
			PasswordConfirmation string `json:"passwordConfirmation" validate:"required"`
		}

		response struct {
			ID string `json:"id"`
		}
	)

	return httputil.NewHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		uniqueID, err := uuidGenerator.GenerateUUIDV4()
		if err != nil {
			logger.ErrorContext(r.Context(), "Failed to generate uuid for new Identity", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		passwordHash, err := NewPasswordHash(r.Data.Password, passwordCost)
		if err != nil {
			logger.ErrorContext(r.Context(), "Failed to create password hash for new Identity", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		identity := Identity{
			ID:       uniqueID,
			Name:     r.Data.Name,
			Email:    r.Data.Email,
			Password: passwordHash,
		}

		if err = identities.Create(r.Context(), identity); err != nil {
			if errors.Is(err, ErrDuplicateIdentity) {
				return nil, problem.ResourceExists(r.Request)
			}

			logger.ErrorContext(r.Context(), "Failed to create new Identity", slog.Any("error", err))

			return nil, problem.ServerError(r.Request)
		}

		logger.InfoContext(r.Context(), "Successfully created new Identity", slog.String("id", identity.ID.String()))

		return httputil.Created(envelope{Data: response{ID: identity.ID.String()}})
	})
}
```

- [x] **Step 4: Update `iam.Endpoints` signature**

Edit `api/internal/iam/endpoints.go`. Replace the existing `Endpoints` with:

```go
// Endpoints returns the EndpointGroup for the IAM API. The jwtKey is used to
// sign and verify JWTs; it must not be empty (validated by the caller).
// passwordCost is forwarded to NewPasswordHash; main.go passes
// bcrypt.DefaultCost. now is the clock used for JWT iat/exp; main.go passes
// time.Now.
func Endpoints(
	logger *slog.Logger,
	uuidV4Generator UUIDV4Generator,
	identityRepository IdentityRepository,
	jwtKey string,
	passwordCost int,
	now func() time.Time,
) httputil.EndpointGroup {
	publicEndpoints := httputil.EndpointGroup{
		{
			Path:    "/identities",
			Method:  http.MethodPost,
			Handler: identityCreateHandler(logger, uuidV4Generator, identityRepository, passwordCost),
		},
		{
			Path:    "/tokens",
			Method:  http.MethodPost,
			Handler: tokenCreateHandler(logger, identityRepository, uuidV4Generator, jwtKey, now),
		},
	}

	authEndpoints := httputil.EndpointGroup{
		{
			Path:    "/identities/me",
			Method:  http.MethodGet,
			Handler: identityMeHandler(logger, identityRepository),
		},
	}.WithGuard(NewJWTGuard(logger, jwtKey))

	endpoints := make(httputil.EndpointGroup, 0, len(publicEndpoints)+len(authEndpoints))
	endpoints = append(endpoints, publicEndpoints...)
	endpoints = append(endpoints, authEndpoints...)

	return endpoints.WithPrefix("/iam")
}
```

Update the imports at the top of `endpoints.go` to add `"time"`:

```go
import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
)
```

- [x] **Step 5: Update `tokenCreateHandler` signature (body unchanged for now)**

Edit `api/internal/iam/token.go`. Change the function signature only — the body still uses `time.Now()` and `uuid.NewRandom()` after this step, which is fine because Task 7 finishes wiring the new params into the body:

```go
func tokenCreateHandler(logger *slog.Logger, identities IdentityRepository, uuidGenerator UUIDV4Generator, jwtKey string, now func() time.Time) http.Handler {
	const oneDay = 24 * time.Hour

	// uuidGenerator and now are wired through here; their bodies are used in Task 7.
	// Keep `_ = ...` to satisfy linters that flag unused parameters until Task 7
	// removes them.
	_ = uuidGenerator
	_ = now

	// ... rest of body unchanged
```

- [x] **Step 6: Update `main.go`**

Edit `api/main.go`. Replace the `server.Register(...)` line:

```go
server.Register(
	iam.Endpoints(logger, uuidV4Generator{}, identityRepository, jwtKey, bcrypt.DefaultCost, time.Now)...,
)
```

Add imports for `time` and `golang.org/x/crypto/bcrypt`:

```go
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
```

- [x] **Step 7: Run the test, expect it to pass**

Run: `go test -race -shuffle=on ./api/internal/iam/... -run TestIdentityCreateHandler_Success`
Expected: PASS.

- [x] **Step 8: Run full build to verify nothing else broke**

Run: `go build ./...`
Expected: clean build.

- [x] **Step 9: Commit**

```bash
git add api/internal/iam/identity.go api/internal/iam/endpoints.go api/internal/iam/token.go api/main.go api/internal/iam/identity_test.go
git commit -m "refactor(api/iam): inject bcrypt cost into Endpoints; add identity success test"
```

---

## Task 7: Plumb UUID generator and clock into tokenCreateHandler

**Files:**
- Modify: `api/internal/iam/token.go`
- Create (append to): `api/internal/iam/token_test.go`

- [x] **Step 1: Write the failing test**

Write `api/internal/iam/token_test.go`:

```go
package iam_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/testutil"
)

type tokenClaims struct {
	jwt.RegisteredClaims

	UUID uuid.UUID `json:"uuid"`
}

func TestTokenCreateHandler_Success(t *testing.T) {
	t.Parallel()

	repo := testutil.NewIdentityRepository()
	repo.Seed(testutil.KnownIdentity())

	jti := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	gen := testutil.NewUUIDV4Generator(jti)

	server := newServer(t, repo, gen)

	body := bytes.NewBufferString(`{
		"email": "known@example.com",
		"password": "correct-horse-battery-staple"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/iam/tokens", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d\nbody: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	parsed, err := jwt.ParseWithClaims(resp.Data.Token, &tokenClaims{}, func(_ *jwt.Token) (any, error) {
		return []byte(testutil.JWTKey), nil
	})
	if err != nil {
		t.Fatalf("parse jwt: %v", err)
	}

	got, ok := parsed.Claims.(*tokenClaims)
	if !ok {
		t.Fatalf("claims type: got %T", parsed.Claims)
	}

	want := &tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "objectory",
			Subject:   "authentication",
			Audience:  jwt.ClaimStrings{"objectory"},
			ExpiresAt: jwt.NewNumericDate(testutil.FixedTime.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(testutil.FixedTime),
			ID:        jti.String(),
		},
		UUID: testutil.KnownIdentityID,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("claims mismatch (-want +got):\n%s", diff)
	}
}
```

Add a helper at the bottom of `identity_test.go` (re-used here):

```go
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
```

…and add `"encoding/json"` to `identity_test.go`'s imports.

- [x] **Step 2: Run the test, expect failure**

Run: `go test -race ./api/internal/iam/... -run TestTokenCreateHandler_Success`
Expected: test runs but fails on the `cmp.Diff` — `IssuedAt` and `ExpiresAt` will be real wall-clock time (not `FixedTime`), and `ID` will be a random UUID (not `jti`).

- [x] **Step 3: Wire the generator and clock into `tokenCreateHandler`**

Edit `api/internal/iam/token.go`. Replace the body's `uuid.NewRandom()` and `time.Now()` calls; remove the `_ = ...` placeholders:

```go
func tokenCreateHandler(logger *slog.Logger, identities IdentityRepository, uuidGenerator UUIDV4Generator, jwtKey string, now func() time.Time) http.Handler {
	const oneDay = 24 * time.Hour

	type (
		request struct {
			Email    string `json:"email"    validate:"required,email"`
			Password string `json:"password" validate:"required,min=8,max=64"`
		}

		response struct {
			Token string `json:"token"`
		}
	)

	return httputil.NewHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		identity, err := identities.FindByEmail(r.Context(), r.Data.Email)
		if errors.Is(err, ErrIdentityNotFound) {
			return nil, problem.Unauthorized(r.Request)
		} else if err != nil {
			logger.WarnContext(r.Context(), "Failed to find identity by email when creating new token", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		if !identity.Password.Matches(r.Data.Password) {
			return nil, problem.Unauthorized(r.Request)
		}

		jtiBytes, err := uuidGenerator.GenerateUUIDV4()
		if err != nil {
			logger.WarnContext(r.Context(), "Failed to generate uuid for jwt token", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}
		jti := uuid.UUID(jtiBytes)

		issuedAt := now()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "objectory",
				Subject:   "authentication",
				Audience:  jwt.ClaimStrings{"objectory"},
				ExpiresAt: jwt.NewNumericDate(issuedAt.Add(oneDay)),
				NotBefore: nil,
				IssuedAt:  jwt.NewNumericDate(issuedAt),
				ID:        jti.String(),
			},
			UUID: identity.ID,
		})

		tokenString, err := token.SignedString([]byte(jwtKey))
		if err != nil {
			logger.WarnContext(r.Context(), "Failed to create signed token string", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		return httputil.Created(envelope{Data: response{Token: tokenString}})
	})
}
```

Replace `"github.com/google/uuid"` import to keep using `uuid.UUID` for the conversion.

- [x] **Step 4: Run the test, expect pass**

Run: `go test -race ./api/internal/iam/... -run TestTokenCreateHandler_Success`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add api/internal/iam/token.go api/internal/iam/token_test.go api/internal/iam/identity_test.go
git commit -m "refactor(api/iam): inject UUID generator and clock into tokenCreateHandler"
```

---

## Task 8: Expand identityCreateHandler tests — error paths

**Files:**
- Modify: `api/internal/iam/identity_test.go`

- [x] **Step 1: Add table-driven validation and error-path tests**

Append to `api/internal/iam/identity_test.go`:

```go
func TestIdentityCreateHandler_Errors(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed       func(*testutil.IdentityRepository)
		body       string
		repoErr    func(*testutil.IdentityRepository)
		uuidErr    error
		wantStatus int
		wantBody   string
	}{
		"missing name": {
			body: `{"email": "x@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/validation-failed.md",
				"title": "The request data was invalid",
				"status": 422,
				"detail": "The request data was invalid, see properties for details",
				"instance": "/iam/identities",
				"properties": [
					{"detail": "name is required", "pointer": "#/name"}
				]
			}`,
		},
		"password too short": {
			body: `{"name": "A", "email": "a@example.com", "password": "short", "passwordConfirmation": "short"}`,
			wantStatus: http.StatusUnprocessableEntity,
			// only key fields asserted; the helper does field-level diff
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/validation-failed.md",
				"title": "The request data was invalid",
				"status": 422,
				"detail": "The request data was invalid, see properties for details",
				"instance": "/iam/identities",
				"properties": [
					{"detail": "password must be at least 8 characters long", "pointer": "#/password"}
				]
			}`,
		},
		"duplicate email": {
			seed: func(r *testutil.IdentityRepository) { r.Seed(testutil.KnownIdentity()) },
			body: `{"name": "A", "email": "known@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			wantStatus: http.StatusConflict,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-exists.md",
				"title": "Resource already exists",
				"status": 409,
				"detail": "The resource you are trying to create already exists",
				"instance": "/iam/identities"
			}`,
		},
		"uuid generation fails": {
			body: `{"name": "A", "email": "a@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			uuidErr: errors.New("entropy exhausted"),
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Internal Server Error",
				"status": 500,
				"detail": "The server encountered a problem and could not process your request",
				"instance": "/iam/identities"
			}`,
		},
		"repository returns unexpected error": {
			body: `{"name": "A", "email": "a@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			repoErr: func(r *testutil.IdentityRepository) { r.CreateErr = errors.New("connection refused") },
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Internal Server Error",
				"status": 500,
				"detail": "The server encountered a problem and could not process your request",
				"instance": "/iam/identities"
			}`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := testutil.NewIdentityRepository()
			if tc.seed != nil {
				tc.seed(repo)
			}
			if tc.repoErr != nil {
				tc.repoErr(repo)
			}

			gen := testutil.NewUUIDV4Generator(uuid.MustParse("33333333-3333-3333-3333-333333333333"))
			gen.Err = tc.uuidErr

			server := newServer(t, repo, gen)

			req := httptest.NewRequest(http.MethodPost, "/iam/identities", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			testutil.ProblemResponse(t, rec, tc.wantStatus, tc.wantBody)
		})
	}
}
```

Add `"errors"` to the imports of `identity_test.go` if not already present.

- [x] **Step 2: Run the tests**

Run: `go test -race -shuffle=on ./api/internal/iam/... -run TestIdentityCreateHandler_Errors`
Expected: PASS for all five cases.

If any case's `wantBody` doesn't match what `httputil/problem` actually emits, update `wantBody` to match the real shape — the helper's `cmp.Diff` failure output will show the exact difference. The shapes above are the documented contract; the test acts as a contract test against `httputil/problem`.

- [x] **Step 3: Commit**

```bash
git add api/internal/iam/identity_test.go
git commit -m "test(api/iam): cover identityCreateHandler error paths"
```

---

## Task 9: Expand tokenCreateHandler tests — error paths

**Files:**
- Modify: `api/internal/iam/token_test.go`

- [x] **Step 1: Add table-driven error tests**

Append to `api/internal/iam/token_test.go`:

```go
func TestTokenCreateHandler_Errors(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed       func(*testutil.IdentityRepository)
		body       string
		repoErr    func(*testutil.IdentityRepository)
		uuidErr    error
		wantStatus int
		wantBody   string
	}{
		"missing email": {
			body: `{"password": "supersecret"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/validation-failed.md",
				"title": "The request data was invalid",
				"status": 422,
				"detail": "The request data was invalid, see properties for details",
				"instance": "/iam/tokens",
				"properties": [
					{"detail": "email is required", "pointer": "#/email"}
				]
			}`,
		},
		"identity not found": {
			body: `{"email": "missing@example.com", "password": "supersecret"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
				"title": "Unauthorized",
				"status": 401,
				"detail": "You must be authenticated to access this resource",
				"instance": "/iam/tokens"
			}`,
		},
		"wrong password": {
			seed: func(r *testutil.IdentityRepository) { r.Seed(testutil.KnownIdentity()) },
			body: `{"email": "known@example.com", "password": "wrong-password"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
				"title": "Unauthorized",
				"status": 401,
				"detail": "You must be authenticated to access this resource",
				"instance": "/iam/tokens"
			}`,
		},
		"repository unexpected error": {
			body: `{"email": "x@example.com", "password": "supersecret"}`,
			repoErr: func(r *testutil.IdentityRepository) { r.FindByEmailErr = errors.New("connection refused") },
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Internal Server Error",
				"status": 500,
				"detail": "The server encountered a problem and could not process your request",
				"instance": "/iam/tokens"
			}`,
		},
		"uuid generation fails": {
			seed: func(r *testutil.IdentityRepository) { r.Seed(testutil.KnownIdentity()) },
			body: `{"email": "known@example.com", "password": "correct-horse-battery-staple"}`,
			uuidErr: errors.New("entropy exhausted"),
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Internal Server Error",
				"status": 500,
				"detail": "The server encountered a problem and could not process your request",
				"instance": "/iam/tokens"
			}`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := testutil.NewIdentityRepository()
			if tc.seed != nil {
				tc.seed(repo)
			}
			if tc.repoErr != nil {
				tc.repoErr(repo)
			}

			gen := testutil.NewUUIDV4Generator(uuid.MustParse("44444444-4444-4444-4444-444444444444"))
			gen.Err = tc.uuidErr

			server := newServer(t, repo, gen)

			req := httptest.NewRequest(http.MethodPost, "/iam/tokens", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			testutil.ProblemResponse(t, rec, tc.wantStatus, tc.wantBody)
		})
	}
}
```

Add `"bytes"`, `"errors"`, `"net/http"`, `"net/http/httptest"`, `"github.com/google/uuid"` imports if not already present in `token_test.go`.

- [x] **Step 2: Run the tests**

Run: `go test -race -shuffle=on ./api/internal/iam/... -run TestTokenCreateHandler_Errors`
Expected: PASS for all five cases. Adjust `wantBody` strings as needed if the actual `httputil/problem` shape differs from the comments above.

- [x] **Step 3: Commit**

```bash
git add api/internal/iam/token_test.go
git commit -m "test(api/iam): cover tokenCreateHandler error paths"
```

---

## Task 10: identityMeHandler unit tests

**Files:**
- Modify: `api/internal/iam/identity_test.go`

- [x] **Step 1: Add the success and error tests**

Append to `api/internal/iam/identity_test.go`:

```go
func TestIdentityMeHandler_Success(t *testing.T) {
	t.Parallel()

	repo := testutil.NewIdentityRepository()
	repo.Seed(testutil.KnownIdentity())

	server := newServer(t, repo, testutil.NewUUIDV4Generator())

	token := signTestJWT(t, testutil.KnownIdentityID)

	req := httptest.NewRequest(http.MethodGet, "/iam/identities/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	testutil.JSONResponse(t, rec, http.StatusOK, `{
		"data": {
			"id": "00000000-0000-0000-0000-000000000001",
			"name": "Known Test User",
			"email": "known@example.com"
		}
	}`)
}

func TestIdentityMeHandler_NotFound(t *testing.T) {
	t.Parallel()

	repo := testutil.NewIdentityRepository()
	// Seed with a different identity so the JWT's UUID is not findable.
	server := newServer(t, repo, testutil.NewUUIDV4Generator())

	token := signTestJWT(t, testutil.KnownIdentityID)

	req := httptest.NewRequest(http.MethodGet, "/iam/identities/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	testutil.ProblemResponse(t, rec, http.StatusNotFound, `{
		"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-not-found.md",
		"title": "Resource not found",
		"status": 404,
		"detail": "The requested resource was not found",
		"instance": "/iam/identities/me"
	}`)
}

// signTestJWT signs a JWT for the given identity using testutil.JWTKey,
// matching the iam package's claim shape. Used by tests that exercise
// authenticated endpoints behind the JWT guard.
func signTestJWT(t *testing.T, identityID uuid.UUID) string {
	t.Helper()

	type testClaims struct {
		jwt.RegisteredClaims
		UUID uuid.UUID `json:"uuid"`
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "objectory",
			Subject:   "authentication",
			Audience:  jwt.ClaimStrings{"objectory"},
			IssuedAt:  jwt.NewNumericDate(testutil.FixedTime),
			ExpiresAt: jwt.NewNumericDate(testutil.FixedTime.Add(24 * time.Hour)),
			ID:        uuid.NewString(),
		},
		UUID: identityID,
	})

	signed, err := tok.SignedString([]byte(testutil.JWTKey))
	if err != nil {
		t.Fatalf("sign test jwt: %v", err)
	}
	return signed
}
```

Add `"time"`, `"github.com/golang-jwt/jwt/v5"`, `"github.com/google/uuid"` to `identity_test.go`'s imports.

- [x] **Step 2: Run the tests**

Run: `go test -race -shuffle=on ./api/internal/iam/... -run TestIdentityMeHandler`
Expected: both pass.

- [x] **Step 3: Commit**

```bash
git add api/internal/iam/identity_test.go
git commit -m "test(api/iam): cover identityMeHandler success and not-found paths"
```

---

## Task 11: NewJWTGuard and CurrentIdentityFromContext tests

**Files:**
- Create: `api/internal/iam/auth_test.go`

- [x] **Step 1: Write the file**

Write `api/internal/iam/auth_test.go`:

```go
package iam_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func TestCurrentIdentityFromContext(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		ctx     context.Context
		wantID  uuid.UUID
		wantOk  bool
	}{
		"empty context returns false": {
			ctx:    context.Background(),
			wantOk: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotID, gotOk := iam.CurrentIdentityFromContext(tc.ctx)
			if gotOk != tc.wantOk {
				t.Errorf("ok: got %v, want %v", gotOk, tc.wantOk)
			}
			if gotID != tc.wantID {
				t.Errorf("id: got %s, want %s", gotID, tc.wantID)
			}
		})
	}
}

// TestNewJWTGuard tests the guard's outcome at the GuardFunc seam: a valid
// token returns (request-with-identity, nil); an invalid token returns
// (nil, error). The rendered HTTP response shape (Forbidden / 403 problem
// JSON) is covered in feature tests, where the guard runs inside the full
// httputil stack.
func TestNewJWTGuard(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		header  string
		wantOK  bool
	}{
		"valid token populates context": {
			header: "Bearer " + signTestJWT(t, testutil.KnownIdentityID),
			wantOK: true,
		},
		"missing authorization header errors": {
			header: "",
			wantOK: false,
		},
		"wrong signing key errors": {
			header: "Bearer " + signJWTWithKey(t, "00000000000000000000000000000000-other"),
			wantOK: false,
		},
		"expired token errors": {
			header: "Bearer " + signExpiredJWT(t),
			wantOK: false,
		},
		"malformed token errors": {
			header: "Bearer not.a.jwt",
			wantOK: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			guard := iam.NewJWTGuard(quietLogger(), testutil.JWTKey)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			passed, err := guard(req)

			if tc.wantOK {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				if passed == nil {
					t.Fatal("expected request, got nil")
				}
				gotID, ok := iam.CurrentIdentityFromContext(passed.Context())
				if !ok {
					t.Fatal("guard did not store identity in context")
				}
				if gotID != testutil.KnownIdentityID {
					t.Errorf("identity mismatch: got %s, want %s", gotID, testutil.KnownIdentityID)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error, got nil; passed = %v", passed)
			}
			if passed != nil {
				t.Errorf("expected nil request on error path, got %v", passed)
			}
		})
	}
}

func signJWTWithKey(t *testing.T, key string) string {
	t.Helper()

	type testClaims struct {
		jwt.RegisteredClaims
		UUID uuid.UUID `json:"uuid"`
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(testutil.FixedTime),
			ExpiresAt: jwt.NewNumericDate(testutil.FixedTime.Add(time.Hour)),
		},
		UUID: testutil.KnownIdentityID,
	})

	signed, err := tok.SignedString([]byte(key))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func signExpiredJWT(t *testing.T) string {
	t.Helper()

	type testClaims struct {
		jwt.RegisteredClaims
		UUID uuid.UUID `json:"uuid"`
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(testutil.FixedTime.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(testutil.FixedTime.Add(-time.Hour)),
		},
		UUID: testutil.KnownIdentityID,
	})

	signed, err := tok.SignedString([]byte(testutil.JWTKey))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}
```

> The rendered 403 problem-JSON shape is covered indirectly by the feature tests in Task 16 (any unauthenticated GET to `/iam/identities/me` exercises the guard's error path through the full httputil stack). The unit test here intentionally stays at the `GuardFunc` seam.

- [x] **Step 2: Run the tests**

Run: `go test -race -shuffle=on ./api/internal/iam/... -run TestNewJWTGuard`
Expected: PASS for valid case; for the invalid cases, the test asserts `err != nil` which is what the guard returns. If you can render the problem response cleanly via httputil, replace those branches with `testutil.ProblemResponse` assertions.

Also run:

Run: `go test -race -shuffle=on ./api/internal/iam/... -run TestCurrentIdentityFromContext`
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add api/internal/iam/auth_test.go
git commit -m "test(api/iam): cover NewJWTGuard and CurrentIdentityFromContext"
```

---

## Task 12: Extract pgxSlogAdapter to api/internal/log/pgxlog

**Files:**
- Create: `api/internal/log/pgxlog/pgxlog.go`
- Create: `api/internal/log/pgxlog/pgxlog_test.go`
- Modify: `api/main.go`

- [x] **Step 1: Write the failing test**

Write `api/internal/log/pgxlog/pgxlog_test.go`:

```go
package pgxlog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/tracelog"

	"github.com/nickbryan/objectory/api/internal/log/pgxlog"
)

func TestAdapter_Log(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		level     tracelog.LogLevel
		msg       string
		data      map[string]any
		wantLevel string
		wantMsg   string
		wantSkip  bool
	}{
		"none level is skipped": {
			level:    tracelog.LogLevelNone,
			msg:      "noisy query",
			wantSkip: true,
		},
		"debug maps to debug": {
			level:     tracelog.LogLevelDebug,
			msg:       "executing query",
			data:      map[string]any{"sql": "SELECT 1"},
			wantLevel: "DEBUG",
			wantMsg:   "executing query",
		},
		"info maps to info": {
			level:     tracelog.LogLevelInfo,
			msg:       "row returned",
			wantLevel: "INFO",
			wantMsg:   "row returned",
		},
		"warn maps to warn": {
			level:     tracelog.LogLevelWarn,
			msg:       "slow query",
			wantLevel: "WARN",
			wantMsg:   "slow query",
		},
		"error maps to error": {
			level:     tracelog.LogLevelError,
			msg:       "query failed",
			wantLevel: "ERROR",
			wantMsg:   "query failed",
		},
		"unknown level maps to error and includes invalid_pgx_log_level": {
			level:     tracelog.LogLevel(99),
			msg:       "unknown",
			wantLevel: "ERROR",
			wantMsg:   "unknown",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			adapter := pgxlog.NewAdapter(logger)

			adapter.Log(context.Background(), tc.level, tc.msg, tc.data)

			if tc.wantSkip {
				if buf.Len() != 0 {
					t.Errorf("expected no log output for level None, got %s", buf.String())
				}
				return
			}

			line := strings.TrimSpace(buf.String())
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("decode log line: %v\nline: %s", err, line)
			}

			if got := entry["level"]; got != tc.wantLevel {
				t.Errorf("level: got %v, want %s", got, tc.wantLevel)
			}
			if got := entry["msg"]; got != tc.wantMsg {
				t.Errorf("msg: got %v, want %s", got, tc.wantMsg)
			}
			if tc.level == tracelog.LogLevel(99) {
				if _, ok := entry["invalid_pgx_log_level"]; !ok {
					t.Errorf("expected invalid_pgx_log_level attr; entry: %v", entry)
				}
			}
		})
	}
}
```

- [x] **Step 2: Run the test, expect failure (package doesn't exist)**

Run: `go test ./api/internal/log/pgxlog/...`
Expected: build error: `package pgxlog is not in std`.

- [x] **Step 3: Create the package**

Write `api/internal/log/pgxlog/pgxlog.go`:

```go
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
```

- [x] **Step 4: Run the test, expect pass**

Run: `go test -race -shuffle=on ./api/internal/log/pgxlog/...`
Expected: PASS for all six cases.

- [x] **Step 5: Update main.go to use the new package**

Edit `api/main.go`. Remove the `pgxSlogAdapter` type and methods at the bottom. Replace the `Tracer` line:

```go
dbConfig.ConnConfig.Tracer = &tracelog.TraceLog{
	Logger:   pgxlog.NewAdapter(logger),
	LogLevel: tracelog.LogLevelInfo,
}
```

Add the import `"github.com/nickbryan/objectory/api/internal/log/pgxlog"`.

- [x] **Step 6: Verify build and existing tests still pass**

Run: `go build ./... && go test -race -shuffle=on ./...`
Expected: clean build; all tests pass.

- [x] **Step 7: Commit**

```bash
git add api/internal/log/pgxlog/ api/main.go
git commit -m "refactor(api): extract pgxSlogAdapter to api/internal/log/pgxlog"
```

---

## Task 13: Extract uuidV4Generator to api/internal/uuidgen

**Files:**
- Create: `api/internal/uuidgen/uuidgen.go`
- Create: `api/internal/uuidgen/uuidgen_test.go`
- Modify: `api/main.go`

- [x] **Step 1: Write the failing test**

Write `api/internal/uuidgen/uuidgen_test.go`:

```go
package uuidgen_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/uuidgen"
)

func TestNew_GenerateUUIDV4_ReturnsValidV4(t *testing.T) {
	t.Parallel()

	gen := uuidgen.New()
	bytes, err := gen.GenerateUUIDV4()
	if err != nil {
		t.Fatalf("GenerateUUIDV4: %v", err)
	}

	got := uuid.UUID(bytes)
	if got.Version() != 4 {
		t.Errorf("version: got %d, want 4", got.Version())
	}
	if got.Variant() != uuid.RFC4122 {
		t.Errorf("variant: got %d, want %d (RFC 4122)", got.Variant(), uuid.RFC4122)
	}
	if got == uuid.Nil {
		t.Error("got nil uuid")
	}
}

func TestNew_GenerateUUIDV4_ReturnsUnique(t *testing.T) {
	t.Parallel()

	gen := uuidgen.New()
	seen := make(map[uuid.UUID]struct{}, 100)
	for range 100 {
		b, err := gen.GenerateUUIDV4()
		if err != nil {
			t.Fatalf("GenerateUUIDV4: %v", err)
		}
		id := uuid.UUID(b)
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate uuid: %s", id)
		}
		seen[id] = struct{}{}
	}
}
```

- [x] **Step 2: Run, expect package-not-found failure**

Run: `go test ./api/internal/uuidgen/...`
Expected: build error.

- [x] **Step 3: Create the package**

Write `api/internal/uuidgen/uuidgen.go`:

```go
// Package uuidgen provides a v4 UUID generator backed by github.com/google/uuid.
// It exists as a small, testable wrapper so that production wiring does not
// inline uuid.NewRandom() calls and so that the package boundary makes the
// dependency explicit at the seam.
package uuidgen

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/iam"
)

// New returns a real iam.UUIDV4Generator that produces cryptographically
// random v4 UUIDs.
func New() iam.UUIDV4Generator {
	return generator{}
}

type generator struct{}

// GenerateUUIDV4 implements iam.UUIDV4Generator.
func (generator) GenerateUUIDV4() ([16]byte, error) {
	next, err := uuid.NewRandom()
	if err != nil {
		return [16]byte{}, fmt.Errorf("creating new random uuid: %w", err)
	}
	return next, nil
}
```

- [x] **Step 4: Run, expect pass**

Run: `go test -race -shuffle=on ./api/internal/uuidgen/...`
Expected: PASS.

- [x] **Step 5: Update main.go**

Edit `api/main.go`. Remove the `uuidV4Generator` type and method at the bottom. Replace the `Endpoints` call:

```go
server.Register(
	iam.Endpoints(logger, uuidgen.New(), identityRepository, jwtKey, bcrypt.DefaultCost, time.Now)...,
)
```

Add the import `"github.com/nickbryan/objectory/api/internal/uuidgen"`. Remove the now-unused `"fmt"` and `"github.com/google/uuid"` imports if no other usage remains.

- [x] **Step 6: Verify build and tests**

Run: `go build ./... && go test -race -shuffle=on ./...`
Expected: clean build; all tests pass.

- [x] **Step 7: Commit**

```bash
git add api/internal/uuidgen/ api/main.go
git commit -m "refactor(api): extract uuidV4Generator to api/internal/uuidgen"
```

---

## Task 14: testutil/postgresdb.go

**Files:**
- Create: `api/internal/testutil/postgresdb.go`

- [x] **Step 1: Write the file**

Write `api/internal/testutil/postgresdb.go`:

```go
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql, used by goose
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	pgmigrations "github.com/nickbryan/objectory/api/internal/storage/postgres/migrations"
)

const (
	templateDBName = "template_iam"
	containerLabel = "objectory-test-postgres"
)

var (
	setupOnce sync.Once
	setupErr  error
	baseDSN   string // "postgres://user:pass@host:port" — no database segment
)

// NewTestDB returns a *pgxpool.Pool connected to a freshly-cloned test
// database. The database is created from a once-migrated template_iam
// database via CREATE DATABASE ... TEMPLATE ..., which Postgres makes cheap.
// On test cleanup the pool is closed and the database is dropped.
func NewTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	setupOnce.Do(func() {
		setupErr = setupTemplate(context.Background())
	})
	if setupErr != nil {
		t.Fatalf("postgres test setup: %v", setupErr)
	}

	dbName := "test_" + randHex(8)

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, baseDSN+"/postgres")
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	defer adminPool.Close()

	createSQL := fmt.Sprintf(`CREATE DATABASE "%s" TEMPLATE "%s"`, dbName, templateDBName)
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
		dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, dbName)
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
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
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
	baseDSN = fmt.Sprintf("postgres://test:test@%s:%s", host, port.Port())

	if err := ensureTemplate(ctx); err != nil {
		return fmt.Errorf("ensure template: %w", err)
	}
	return nil
}

func ensureTemplate(ctx context.Context) error {
	adminPool, err := pgxpool.New(ctx, baseDSN+"/postgres")
	if err != nil {
		return fmt.Errorf("connect admin pool: %w", err)
	}
	defer adminPool.Close()

	var exists bool
	err = adminPool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", templateDBName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check template existence: %w", err)
	}

	if !exists {
		// Race-tolerant: another process may create it concurrently.
		if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, templateDBName)); err != nil {
			// Ignore "already exists" — concurrent creation is fine. Surface anything else.
			// Postgres returns SQLSTATE 42P04 (duplicate_database) for this.
			// pgx surfaces this as a *pgconn.PgError; rather than depending on the package
			// here, we re-check existence and only fail if it still doesn't exist.
			var stillExists bool
			adminPool.QueryRow(ctx,
				"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", templateDBName).Scan(&stillExists)
			if !stillExists {
				return fmt.Errorf("create template db: %w", err)
			}
		}
	}

	db, err := sql.Open("pgx", baseDSN+"/"+templateDBName)
	if err != nil {
		return fmt.Errorf("open template db: %w", err)
	}
	defer db.Close()

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
```

- [x] **Step 2: Verify build**

Run: `go build ./api/internal/testutil/...`
Expected: clean build.

- [x] **Step 3: Commit**

```bash
git add api/internal/testutil/postgresdb.go
git commit -m "test(testutil): add Postgres testcontainer + per-test database lifecycle"
```

---

## Task 15: Storage repository integration tests

**Files:**
- Create: `api/internal/storage/identity_integration_test.go`

- [ ] **Step 1: Write the integration test file**

Write `api/internal/storage/identity_integration_test.go`:

```go
//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/storage"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func TestIdentityRepository_Create(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed    func(*testing.T, *storage.IdentityRepository)
		input   iam.Identity
		wantErr error
	}{
		"creates a new identity": {
			input: iam.Identity{
				ID:       testutil.KnownIdentityID,
				Name:     "Alice",
				Email:    "alice@example.com",
				Password: iam.NewHashedPassword([]byte("hashed")),
			},
		},
		"duplicate email returns ErrDuplicateIdentity": {
			seed: func(t *testing.T, r *storage.IdentityRepository) {
				t.Helper()
				err := r.Create(context.Background(), iam.Identity{
					ID:       testutil.KnownIdentityID,
					Name:     "First",
					Email:    "dup@example.com",
					Password: iam.NewHashedPassword([]byte("hashed")),
				})
				if err != nil {
					t.Fatalf("seed: %v", err)
				}
			},
			input: iam.Identity{
				ID:       uuid.MustParse("99999999-9999-9999-9999-999999999999"), // different ID, same email — exercises email-unique constraint
				Name:     "Second",
				Email:    "dup@example.com",
				Password: iam.NewHashedPassword([]byte("hashed")),
			},
			wantErr: iam.ErrDuplicateIdentity,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := testutil.NewTestDB(t)
			repo := storage.NewIdentityRepository(postgres.New(pool), testutil.Clock())

			if tc.seed != nil {
				tc.seed(t, repo)
			}

			err := repo.Create(context.Background(), tc.input)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}

			if tc.wantErr != nil {
				return
			}

			// Read back via raw SQL to verify timestamps and column shape.
			var (
				gotID        pgtype.UUID
				gotEmail     string
				gotName      string
				gotPassword  string
				gotCreatedAt pgtype.Timestamp
				gotUpdatedAt pgtype.Timestamp
			)
			err = pool.QueryRow(context.Background(),
				`SELECT id, email, name, password, created_at, updated_at FROM iam.identities WHERE id = $1`,
				pgtype.UUID{Bytes: tc.input.ID, Valid: true},
			).Scan(&gotID, &gotEmail, &gotName, &gotPassword, &gotCreatedAt, &gotUpdatedAt)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}

			if diff := cmp.Diff(tc.input.Email, gotEmail); diff != "" {
				t.Errorf("email mismatch:\n%s", diff)
			}
			if diff := cmp.Diff(tc.input.Name, gotName); diff != "" {
				t.Errorf("name mismatch:\n%s", diff)
			}
			if !gotCreatedAt.Time.Equal(testutil.FixedTime) {
				t.Errorf("created_at: got %v, want %v", gotCreatedAt.Time, testutil.FixedTime)
			}
			if !gotUpdatedAt.Time.Equal(testutil.FixedTime) {
				t.Errorf("updated_at: got %v, want %v", gotUpdatedAt.Time, testutil.FixedTime)
			}
		})
	}
}

func TestIdentityRepository_Find(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed    func(*testing.T, *storage.IdentityRepository)
		findID  uuid.UUID
		wantErr error
		wantOK  bool
	}{
		"finds an existing identity": {
			seed: func(t *testing.T, r *storage.IdentityRepository) {
				t.Helper()
				if err := r.Create(context.Background(), testutil.KnownIdentity()); err != nil {
					t.Fatalf("seed: %v", err)
				}
			},
			findID: testutil.KnownIdentityID,
			wantOK: true,
		},
		"returns ErrIdentityNotFound when missing": {
			findID:  testutil.KnownIdentityID,
			wantErr: iam.ErrIdentityNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := testutil.NewTestDB(t)
			repo := storage.NewIdentityRepository(postgres.New(pool), testutil.Clock())

			if tc.seed != nil {
				tc.seed(t, repo)
			}

			got, err := repo.Find(context.Background(), tc.findID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}
			if tc.wantOK {
				if got == nil {
					t.Fatal("expected identity, got nil")
				}
				if got.Email != "known@example.com" {
					t.Errorf("email: got %s, want known@example.com", got.Email)
				}
			}
		})
	}
}

func TestIdentityRepository_FindByEmail(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed       func(*testing.T, *storage.IdentityRepository)
		email      string
		wantErr    error
		wantOK     bool
	}{
		"finds an existing identity by email": {
			seed: func(t *testing.T, r *storage.IdentityRepository) {
				t.Helper()
				if err := r.Create(context.Background(), testutil.KnownIdentity()); err != nil {
					t.Fatalf("seed: %v", err)
				}
			},
			email:  "known@example.com",
			wantOK: true,
		},
		"returns ErrIdentityNotFound when missing": {
			email:   "missing@example.com",
			wantErr: iam.ErrIdentityNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := testutil.NewTestDB(t)
			repo := storage.NewIdentityRepository(postgres.New(pool), testutil.Clock())

			if tc.seed != nil {
				tc.seed(t, repo)
			}

			got, err := repo.FindByEmail(context.Background(), tc.email)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}
			if tc.wantOK {
				if got == nil {
					t.Fatal("expected identity, got nil")
				}
				if got.ID != testutil.KnownIdentityID {
					t.Errorf("id: got %s, want %s", got.ID, testutil.KnownIdentityID)
				}
			}
		})
	}
}
```

Add `"github.com/google/uuid"` to imports.


- [ ] **Step 2: Run the integration suite**

Run: `go test -race -shuffle=on -tags=integration ./api/internal/storage/...`
Expected: PASS for all cases. Docker daemon must be reachable; if not, the testcontainer setup fails fast with a clear error.

- [ ] **Step 3: Commit**

```bash
git add api/internal/storage/identity_integration_test.go
git commit -m "test(api/storage): integration tests for IdentityRepository"
```

---

## Task 16: Feature tests in api/tests/

**Files:**
- Create: `api/tests/identities_test.go`
- Create: `api/tests/tokens_test.go`

- [ ] **Step 1: Write the identities feature tests**

Write `api/tests/identities_test.go`:

```go
//go:build integration

package tests

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/storage"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
	"github.com/nickbryan/objectory/api/internal/testutil"
	"github.com/nickbryan/objectory/api/internal/uuidgen"
)

// newWiredServer returns a server wired with the same iam.Endpoints as
// production, backed by a fresh test database. The returned pool inspects the
// same database the server writes to; both share the lifetime of t.
func newWiredServer(t *testing.T) (*httputil.Server, *storage.IdentityRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewTestDB(t)
	queries := postgres.New(pool)
	repo := storage.NewIdentityRepository(queries, testutil.Clock())

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	server := httputil.NewServer(logger)
	server.Register(iam.Endpoints(
		logger,
		uuidgen.New(),
		repo,
		testutil.JWTKey,
		bcrypt.MinCost,
		testutil.Clock(),
	)...)

	return server, repo, pool
}

func TestRegister_CreatesIdentityAndPersistsHashedPassword(t *testing.T) {
	t.Parallel()

	server, _, pool := newWiredServer(t)

	body := bytes.NewBufferString(`{
		"name": "Alice",
		"email": "alice@example.com",
		"password": "supersecret",
		"passwordConfirmation": "supersecret"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/iam/identities", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rec.Code, rec.Body.String())
	}

	var (
		gotEmail    string
		gotName     string
		gotPassword string
	)
	if err := pool.QueryRow(context.Background(),
		`SELECT email, name, password FROM iam.identities WHERE email = $1`,
		"alice@example.com",
	).Scan(&gotEmail, &gotName, &gotPassword); err != nil {
		t.Fatalf("read back: %v", err)
	}

	if gotEmail != "alice@example.com" {
		t.Errorf("email mismatch: got %q", gotEmail)
	}
	if gotName != "Alice" {
		t.Errorf("name mismatch: got %q", gotName)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(gotPassword), []byte("supersecret")); err != nil {
		t.Errorf("password hash does not match plaintext: %v", err)
	}
}

func TestRegister_DuplicateEmailReturns409(t *testing.T) {
	t.Parallel()

	server, _, _ := newWiredServer(t)

	first := bytes.NewBufferString(`{
		"name": "First",
		"email": "dup@example.com",
		"password": "supersecret",
		"passwordConfirmation": "supersecret"
	}`)
	req1 := httptest.NewRequest(http.MethodPost, "/iam/identities", first)
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	server.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first registration: status %d\nbody: %s", rec1.Code, rec1.Body.String())
	}

	second := bytes.NewBufferString(`{
		"name": "Second",
		"email": "dup@example.com",
		"password": "supersecret",
		"passwordConfirmation": "supersecret"
	}`)
	req2 := httptest.NewRequest(http.MethodPost, "/iam/identities", second)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	server.ServeHTTP(rec2, req2)

	testutil.ProblemResponse(t, rec2, http.StatusConflict, `{
		"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-exists.md",
		"title": "Resource already exists",
		"status": 409,
		"detail": "The resource you are trying to create already exists",
		"instance": "/iam/identities"
	}`)
}
```

The helper returns the `*pgxpool.Pool` so tests inspect the same database the server wrote to. The pool's lifetime is tied to `t` via `testutil.NewTestDB`'s cleanup, so no explicit teardown is needed.

- [ ] **Step 2: Write the tokens / identity-me feature tests**

Write `api/tests/tokens_test.go`:

```go
//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func TestLogin_ReturnsTokenForValidCredentials(t *testing.T) {
	t.Parallel()

	server, repo, _ := newWiredServer(t)

	// Seed an identity directly via the repository so we know the password hash.
	if err := repo.Create(context.Background(), testutil.KnownIdentity()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := bytes.NewBufferString(`{
		"email": "known@example.com",
		"password": "correct-horse-battery-staple"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/iam/tokens", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestLogin_WrongPasswordReturns401(t *testing.T) {
	t.Parallel()

	server, repo, _ := newWiredServer(t)

	if err := repo.Create(context.Background(), testutil.KnownIdentity()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := bytes.NewBufferString(`{
		"email": "known@example.com",
		"password": "wrong"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/iam/tokens", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	testutil.ProblemResponse(t, rec, http.StatusUnauthorized, `{
		"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
		"title": "Unauthorized",
		"status": 401,
		"detail": "You must be authenticated to access this resource",
		"instance": "/iam/tokens"
	}`)
}

func TestMe_ReturnsCurrentIdentityForValidToken(t *testing.T) {
	t.Parallel()

	server, repo, _ := newWiredServer(t)

	if err := repo.Create(context.Background(), testutil.KnownIdentity()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Obtain a real token by calling the login endpoint.
	loginReq := httptest.NewRequest(http.MethodPost, "/iam/tokens",
		bytes.NewBufferString(`{"email": "known@example.com", "password": "correct-horse-battery-staple"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	server.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusCreated {
		t.Fatalf("login: status %d\nbody: %s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/iam/identities/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	meRec := httptest.NewRecorder()
	server.ServeHTTP(meRec, meReq)

	testutil.JSONResponse(t, meRec, http.StatusOK, `{
		"data": {
			"id": "00000000-0000-0000-0000-000000000001",
			"name": "Known Test User",
			"email": "known@example.com"
		}
	}`)

	_ = iam.ErrIdentityNotFound // keep imports honest
}
```

- [ ] **Step 3: Run the feature suite**

Run: `go test -race -shuffle=on -tags=integration ./api/tests/...`
Expected: all tests PASS.

- [ ] **Step 4: Run the full integration suite to confirm cross-package coexistence**

Run: `go test -race -shuffle=on -tags=integration ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add api/tests/
git commit -m "test(api): feature tests for register, login, and identity-me flows"
```

---

## Task 17: Make targets and .gitignore

**Files:**
- Modify: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 1: Replace the test target in the Makefile**

Edit `Makefile`. Remove the existing `testdbname=...` line and the `test:` block. Replace with:

```make
test: ##@Test Run unit tests (no docker, no integration tag)
	go test -race -shuffle=on ./...

test-integration: ##@Test Run unit and integration tests (boots a Postgres testcontainer)
	go test -race -shuffle=on -tags=integration ./...

test-cover: ##@Test Run all tests with coverage; produces coverage.out and coverage.html
	go test -race -shuffle=on -tags=integration \
	    -coverprofile=coverage.out \
	    -coverpkg=./api/internal/iam,./api/internal/storage,./api/internal/log/pgxlog,./api/internal/uuidgen \
	    ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
```

- [ ] **Step 2: Update .gitignore**

Edit `.gitignore` (or create if missing). Append:

```
coverage.out
coverage.html
```

- [ ] **Step 3: Verify the targets work**

Run: `make test`
Expected: all unit tests pass.

Run: `make test-integration`
Expected: all unit + integration tests pass (Docker daemon required).

Run: `make test-cover`
Expected: all tests pass; `coverage.out` written; `coverage.html` written; coverage summary printed to stdout.

- [ ] **Step 4: Commit**

```bash
git add Makefile .gitignore
git commit -m "build: add test, test-integration, and test-cover make targets"
```

---

## Task 18: ADRs

**Files:**
- Create: `docs/adr/0002-test-taxonomy.md`
- Create: `docs/adr/0003-postgres-test-isolation.md`
- Create: `docs/adr/0004-testing-conventions.md`
- Create: `docs/adr/0005-iam-configurable-seams.md`

- [ ] **Step 1: Write ADR-0002**

Write `docs/adr/0002-test-taxonomy.md`:

```markdown
# ADR-0002: Test taxonomy and build-tagged integration tests

## Date

2026-05-09

## Context

The API needs unit tests for handler logic, integration tests that exercise the storage layer against real Postgres, and feature tests that drive the wired-up server end-to-end. Where each lives — and how they are gated — affects developer dev-loop time and CI shape.

## Decision

Three test homes:

1. **Unit tests** — `*_test.go` co-located with production code in `api/internal/iam`, no build tag. Run by plain `go test ./...`. Use hand-rolled fakes from `api/internal/testutil`. No Docker, no DB.
2. **Repository integration tests** — `*_integration_test.go` co-located with `api/internal/storage`, gated by `//go:build integration`. Run against a real Postgres testcontainer. Run via `go test -tags=integration ./...`.
3. **Feature tests** — `api/tests/<topic>_test.go`, gated by `//go:build integration`. Wire up the real handlers + real storage + real Postgres; assert HTTP responses and DB side effects.

Test packages always use the `_test` suffix (e.g. `iam_test`, `storage_test`, `tests`) so tests can only access the package's public API.

## Rationale

- **Unit tests stay fast** — no Docker, instantly runnable.
- **Build tag on integration tests** — `go test ./...` doesn't pull in Docker dependencies for routine inner-loop work, but CI runs `-tags=integration` for full coverage.
- **Feature tests live outside `api/internal/`** — the package boundary mechanically enforces public-API-only testing. Placing them in `api/tests/` reflects their cross-cutting nature ("the system as a whole works") rather than tying them to one package.
- **Three layers, not two** — repository tests target the storage layer directly, while feature tests target the wired server. Conflating them would either over-test storage details from the HTTP layer or under-test the wiring at the storage layer.

## Consequences

- New packages adopt the same pattern: unit tests co-located, integration tests behind `//go:build integration`, feature tests in `api/tests/`.
- CI must run `make test-integration` to cover the build-tagged tests.
- Developers iterating on a single package use `go test ./<pkg>/...` for fast feedback; full integration runs are explicit.
```

- [ ] **Step 2: Write ADR-0003**

Write `docs/adr/0003-postgres-test-isolation.md`:

```markdown
# ADR-0003: Postgres test isolation via per-test database from a migrated template

## Date

2026-05-09

## Context

Integration and feature tests need a real Postgres database. Tests must run with `t.Parallel()` everywhere, so each test needs its own isolated DB state. Options considered: per-test transaction rollback, per-test schema with shared search_path, truncate-between-tests on a single DB, per-test database cloned from a template.

## Decision

Use **per-test database cloned from a once-migrated `template_iam`**:

- A single Postgres testcontainer boots once, shared across test processes via `testcontainers.WithReuseByName`.
- A `sync.Once` lazy init creates `template_iam` and applies goose migrations to it (idempotent — goose's own version table handles concurrent migration attempts).
- `testutil.NewTestDB(t)` runs `CREATE DATABASE test_<random> TEMPLATE template_iam`, returns a connected `*pgxpool.Pool`, and registers `t.Cleanup` to close the pool and `DROP DATABASE`.

Migrations are embedded in the migrations package itself (`api/internal/storage/postgres/migrations/migrations.go`) and exposed as `migrations.FS`, used by both the test runner and (potentially) production startup.

## Rationale

- **Full parallel isolation** — each test gets a fresh, independent database. No mutex over a shared DB; no transaction scoping that would conflict with code that manages its own transactions.
- **Fast** — Postgres clones from a template in tens of milliseconds. Migrations run once per process, not per test.
- **Realistic** — tests exercise the same migrations production runs, with no parallel SQL definitions to drift.
- **Container reuse** — `Reuse: true` keeps the testcontainer warm across `go test` invocations, saving ~3s per cold dev-loop run.

## Consequences

- Tests require Docker daemon access. CI runners without privileged sockets need `TESTCONTAINERS_RYUK_DISABLED=true`.
- The `migrations` Go package becomes a public-ish dependency for the test runner — production may also adopt it for in-process startup migrations later.
- Per-test DB names use a random hex suffix; cross-process collision risk is negligible.
- If a test leaks (panic mid-cleanup), an orphan `test_<random>` database remains. The reaper container cleans it up after container idle timeout, or it survives until next manual cleanup. Acceptable.
```

- [ ] **Step 3: Write ADR-0004**

Write `docs/adr/0004-testing-conventions.md`:

```markdown
# ADR-0004: Testing conventions for the API module

## Date

2026-05-09

## Context

We are establishing the project's first test suite. The conventions chosen here will set expectations for every subsequent test author. The user has been explicit about what they want — and what they don't want — based on past experience with mocking-framework rot, fragile assertions, and slow suites.

## Decision

The following conventions apply to all tests under `api/`:

1. **Hand-rolled fakes only.** No mocking frameworks (`testify/mock`, `gomock`, etc.). Fakes live in `api/internal/testutil` and follow a hybrid pattern: an in-memory implementation that mirrors real behaviour by default, with public fields for per-test error injection.
2. **Comparisons use `github.com/google/go-cmp/cmp` only.** No `reflect.DeepEqual`, no `testify/assert`.
3. **Response bodies asserted as JSON literals.** The `testutil.JSONResponse` and `testutil.ProblemResponse` helpers take a `wantJSON string`, decode both the actual and expected to `any`, and `cmp.Diff` the result. Tests read like the wire contract.
4. **Table-driven cases are `map[string]struct{...}`.** Nested `t.Run(name, …)`. When tabular form hurts readability (multi-step flows, sequenced setup), nested `t.Run` calls without a table are fine.
5. **Public-API testing only.** Test packages use the `_test` suffix (`iam_test`, `storage_test`, `tests`) so they can only see exported identifiers.
6. **Determinism via injection, not fuzziness.** When code reads time, UUIDs, or random values, the test injects a deterministic source via the package's seam (clock function, UUID generator, etc.). No `time.Now() +/- 5s` tolerance assertions.
7. **`t.Parallel()` everywhere.** Tests must not depend on order; `-shuffle=on` is the default Make target.

## Rationale

Each rule prevents a specific failure mode:

- **No mocking frameworks**: removes the "fake matched but real diverged" failure class. Hand-rolled fakes that wrap the production interface stay synchronised by the compiler.
- **`cmp.Diff` everywhere**: a single, readable diff format. `testify/assert` produces inconsistent failure output across packages.
- **JSON-literal assertions**: matches the wire format the API documents publish. Reviewers don't need to mentally translate Go map literals.
- **`map[string]struct{...}`**: random iteration order surfaces ordering bugs immediately. Slice-of-cases would let an ordering bug slip through.
- **Public API only**: tests stay aligned with consumer reality; refactors of unexported helpers don't break the suite.
- **Determinism**: brittleness from `time.Now()` and random UUIDs is the single largest source of flaky tests in a JWT/identity codebase.

## Consequences

- All future packages follow these conventions. Linters (`golangci-lint`) should enforce no `testify/mock` imports.
- New helpers added to `testutil` should follow the hybrid in-memory + error-injection shape unless there's a specific reason not to.
- If a test ever needs fuzzy time matching (e.g. asserting "this happened at most N seconds ago"), prefer fixing the production seam first. Only fall back to fuzziness when injection genuinely cannot reach the call site.
```

- [ ] **Step 4: Write ADR-0005**

Write `docs/adr/0005-iam-configurable-seams.md`:

```markdown
# ADR-0005: Configurable seams in `iam.Endpoints`

## Date

2026-05-09

## Context

The `iam` package's handlers depend on three sources of nondeterminism: the current time (used for JWT `iat`/`exp` and DB timestamps), random UUIDs (identity IDs and JWT `jti`), and bcrypt cost (which is wall-clock-bound at default cost). Tests need each of these to be deterministic; production wants real values.

Two design directions were available: (a) inject each as a parameter at the package's public constructor, or (b) keep production code reaching for global `time.Now`/`uuid.NewRandom` and use `testing/synctest` plus precomputed test fixtures to control behaviour.

## Decision

Inject all three at `iam.Endpoints`:

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

Same `UUIDV4Generator` and `now` are plumbed into both `identityCreateHandler` and `tokenCreateHandler`.

The `storage.IdentityRepository` constructor already takes a `now func()`; the same convention now applies to `iam.Endpoints`.

## Rationale

- **One mechanism across the codebase.** Both `iam` and `storage` constructors take `now func() time.Time`; new packages will follow. A consistent pattern is easier to learn than a mix of injection here / `synctest` bubble there.
- **No durably-blocked-goroutine caveats.** `testing/synctest` doesn't treat real network I/O as durably blocked, so it can't be used in DB-touching tests. Injection works at every layer.
- **Deterministic JWT assertions.** Tests compare entire JWT claim structs with `cmp.Diff` against an exact expected value, including `iat`, `exp`, and `jti` — caught by clock and UUID injection respectively.
- **Bcrypt cost as a knob.** `bcrypt.MinCost` (4) takes ~1ms per hash; `bcrypt.DefaultCost` (10) takes ~75ms. For ~30 hashes across the integration suite, that's ~2s saved per CI run.

## Consequences

- `iam.Endpoints` has six parameters. At the edge of comfortable; if a future addition pushes to seven, collapse into a `Config` struct.
- `main.go` is the single place that passes the production values (`time.Now`, `bcrypt.DefaultCost`, `uuidgen.New()`). Code review on changes to those lines provides the safety net for regressions.
- Tests pass `bcrypt.MinCost` directly; fixtures pre-compute hashes at the same cost so password-matching round-trips work without paying the default-cost penalty.
- `synctest` remains available as a tool for future tests that need to *advance* fake time mid-test (rate-limit windows, refresh-token expiry tests). It's not the default approach for "what value gets stamped here."
```

- [ ] **Step 5: Verify ADRs render**

Run: `ls docs/adr/`
Expected: `0001-...md`, `0002-...md`, `0003-...md`, `0004-...md`, `0005-...md`.

- [ ] **Step 6: Commit**

```bash
git add docs/adr/
git commit -m "docs(adr): record test taxonomy, isolation, conventions, and iam seams"
```

---

## Final verification

- [ ] **Step 1: Run the full test matrix**

```bash
make test
make test-integration
make test-cover
```

Expected: all targets succeed. Inspect `coverage.html` to confirm `iam`, `storage`, `pgxlog`, and `uuidgen` are at or above 90%.

- [ ] **Step 2: Confirm `go vet` passes**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 3: Confirm linter passes**

Run: `make lint`
Expected: no findings (or only pre-existing findings unrelated to test code).

- [ ] **Step 4: Update TODO.md**

Remove the implicit "no tests anywhere" gap from `TODO.md` if it was noted. The auth-tests work removes a major prerequisite for several security-hardening items already listed there; leave those as-is.

```bash
git status
```
Expected: working tree clean if no further changes needed.

---

## Self-review checklist

Before handing this plan off, the implementer should confirm:

- [ ] Every task ends with a single commit. No multi-task commits.
- [ ] Every test file uses `_test` package suffix.
- [ ] Every test calls `t.Parallel()`.
- [ ] No use of `testify/assert` or any mocking framework crept in.
- [ ] `cmp.Diff` is the only comparison helper.
- [ ] `iam.Endpoints` callers (production + tests) all pass the same six arguments in the same order.
- [ ] `coverage.html` shows ≥90% on `iam`, `storage`, `pgxlog`, `uuidgen`.
- [ ] No `time.Now()` or `uuid.NewRandom()` calls remain inside `iam` package code (only in `main.go` and `uuidgen`).
