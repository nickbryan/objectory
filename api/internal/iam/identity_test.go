package iam_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func newServer(t *testing.T, repo iam.IdentityRepository, gen iam.UUIDV4Generator) (*httputil.Server, *slogmem.LoggedRecords) {
	t.Helper()

	logger, records := slogutil.NewInMemoryLogger(slog.LevelDebug)
	server := httputil.NewServer(logger)
	server.Register(iam.Endpoints(
		logger,
		gen,
		repo,
		testutil.JWTKey,
		bcrypt.MinCost,
		testutil.Clock(),
	)...)

	return server, records
}

func TestIdentityCreateHandler_Success(t *testing.T) {
	t.Parallel()

	repo := testutil.NewIdentityRepository()
	newID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	gen := testutil.NewUUIDV4Generator(newID)

	server, records := newServer(t, repo, gen)

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

	if ok, diff := records.Contains(slogmem.RecordQuery{
		Level:   slog.LevelInfo,
		Message: "Successfully created new Identity",
		Attrs:   map[string]slog.Value{"id": slog.StringValue(newID.String())},
	}); !ok {
		t.Errorf("expected info log for successful creation\n%s", diff)
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
		wantLog    *slogmem.RecordQuery
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
			wantLog: &slogmem.RecordQuery{
				Level:   slog.LevelError,
				Message: "Failed to generate uuid for new Identity",
			},
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
			wantLog: &slogmem.RecordQuery{
				Level:   slog.LevelError,
				Message: "Failed to create new Identity",
			},
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

			server, records := newServer(t, repo, gen)

			req := httptest.NewRequest(http.MethodPost, "/iam/identities", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			testutil.ProblemResponse(t, rec, tc.wantStatus, tc.wantBody)

			if tc.wantLog != nil {
				if ok, diff := records.Contains(*tc.wantLog); !ok {
					t.Errorf("expected log %+v\n%s", *tc.wantLog, diff)
				}
			}
		})
	}
}

func TestIdentityMeHandler_Success(t *testing.T) {
	t.Parallel()

	repo := testutil.NewIdentityRepository()
	repo.Seed(testutil.KnownIdentity())

	server, _ := newServer(t, repo, testutil.NewUUIDV4Generator())

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
	server, _ := newServer(t, repo, testutil.NewUUIDV4Generator())

	token := signTestJWT(t, testutil.KnownIdentityID)

	req := httptest.NewRequest(http.MethodGet, "/iam/identities/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	testutil.ProblemResponse(t, rec, http.StatusNotFound, `{
		"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/not-found.md",
		"title": "Not Found",
		"status": 404,
		"code": "404-01",
		"detail": "The requested resource was not found",
		"instance": "/iam/identities/me"
	}`)
}

// signTestJWT signs a JWT for the given identity using testutil.JWTKey,
// matching the iam package's claim shape. Used by tests that exercise
// authenticated endpoints behind the JWT guard.
//
// Issued-at and expires-at are anchored to time.Now() (not testutil.FixedTime)
// because the production JWT guard validates exp against the wall clock; a
// fixed-time helper would silently expire and start failing tests.
func signTestJWT(t *testing.T, identityID uuid.UUID) string {
	t.Helper()

	type testClaims struct {
		jwt.RegisteredClaims

		UUID uuid.UUID `json:"uuid"`
	}

	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "objectory",
			Subject:   "authentication",
			Audience:  jwt.ClaimStrings{"objectory"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
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
