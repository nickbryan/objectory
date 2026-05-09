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
