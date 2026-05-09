package iam_test

import (
	"bytes"
	"errors"
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
			body:       `{"email": "x@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md",
				"title": "Constraint Violation",
				"status": 422,
				"code": "422-02",
				"detail": "The request data violated one or more validation constraints",
				"instance": "/iam/identities",
				"violations": [
					{"detail": "is required", "pointer": "/name"}
				]
			}`,
		},
		"password too short": {
			body:       `{"name": "A", "email": "a@example.com", "password": "short", "passwordConfirmation": "short"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md",
				"title": "Constraint Violation",
				"status": 422,
				"code": "422-02",
				"detail": "The request data violated one or more validation constraints",
				"instance": "/iam/identities",
				"violations": [
					{"detail": "should be min=8", "pointer": "/password"}
				]
			}`,
		},
		"duplicate email": {
			seed:       func(r *testutil.IdentityRepository) { r.Seed(testutil.KnownIdentity()) },
			body:       `{"name": "A", "email": "known@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			wantStatus: http.StatusConflict,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-exists.md",
				"title": "Resource Exists",
				"status": 409,
				"code": "409-01",
				"detail": "A resource already exists with the specified identifier",
				"instance": "/iam/identities"
			}`,
		},
		"uuid generation fails": {
			body:       `{"name": "A", "email": "a@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			uuidErr:    errors.New("entropy exhausted"),
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Server Error",
				"status": 500,
				"code": "500-01",
				"detail": "The server encountered an unexpected internal error",
				"instance": "/iam/identities"
			}`,
		},
		"repository returns unexpected error": {
			body:       `{"name": "A", "email": "a@example.com", "password": "supersecret", "passwordConfirmation": "supersecret"}`,
			repoErr:    func(r *testutil.IdentityRepository) { r.CreateErr = errors.New("connection refused") },
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Server Error",
				"status": 500,
				"code": "500-01",
				"detail": "The server encountered an unexpected internal error",
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
