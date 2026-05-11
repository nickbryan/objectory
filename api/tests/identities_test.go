//go:build integration

package tests

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"

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

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
	server := httputil.NewServer(logger)
	// Feature tests use the real wall clock for iam.Endpoints because the JWT
	// guard validates iat/exp against time.Now() internally; a fixed clock
	// would issue tokens outside the guard's acceptance window.
	server.Register(iam.Endpoints(
		logger,
		uuidgen.New(),
		repo,
		testutil.JWTKey,
		bcrypt.MinCost,
		time.Now,
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
		"title": "Resource Exists",
		"status": 409,
		"code": "409-01",
		"detail": "A resource already exists with the specified identifier",
		"instance": "/iam/identities"
	}`)
}
