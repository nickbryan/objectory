package iam_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/nickbryan/slogutil/slogmem"

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

	server, _ := newServer(t, repo, gen)

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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Validate exp/iat against the same fixed clock the handler signed with;
	// otherwise this test silently expires once the wall clock passes
	// FixedTime + 24h.
	parsed, err := jwt.ParseWithClaims(resp.Data.Token, &tokenClaims{}, func(_ *jwt.Token) (any, error) {
		return []byte(testutil.JWTKey), nil
	}, jwt.WithTimeFunc(testutil.Clock()))
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

func TestTokenCreateHandler_Errors(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("connection refused")
	errUUID := errors.New("entropy exhausted")

	cases := map[string]struct {
		seed       func(*testutil.IdentityRepository)
		body       string
		repoErr    func(*testutil.IdentityRepository)
		uuidErr    error
		wantStatus int
		wantBody   string
		wantLog    *slogmem.RecordQuery
	}{
		"missing email": {
			body:       `{"password": "supersecret"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md",
				"title": "Constraint Violation",
				"status": 422,
				"code": "422-02",
				"detail": "The request data violated one or more validation constraints",
				"instance": "/iam/tokens",
				"violations": [
					{"detail": "is required", "pointer": "/email"}
				]
			}`,
		},
		"identity not found": {
			body:       `{"email": "missing@example.com", "password": "supersecret"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
				"title": "Unauthorized",
				"status": 401,
				"code": "401-01",
				"detail": "You must be authenticated to POST this resource",
				"instance": "/iam/tokens"
			}`,
		},
		"wrong password": {
			seed:       func(r *testutil.IdentityRepository) { r.Seed(testutil.KnownIdentity()) },
			body:       `{"email": "known@example.com", "password": "wrong-password"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
				"title": "Unauthorized",
				"status": 401,
				"code": "401-01",
				"detail": "You must be authenticated to POST this resource",
				"instance": "/iam/tokens"
			}`,
		},
		"repository unexpected error": {
			body:       `{"email": "x@example.com", "password": "supersecret"}`,
			repoErr:    func(r *testutil.IdentityRepository) { r.FindByEmailErr = errRepo },
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Server Error",
				"status": 500,
				"code": "500-01",
				"detail": "The server encountered an unexpected internal error",
				"instance": "/iam/tokens"
			}`,
			wantLog: &slogmem.RecordQuery{
				Level:   slog.LevelWarn,
				Message: "Failed to find identity by email when creating new token",
				Attrs:   map[string]slog.Value{"error": slog.AnyValue(errRepo)},
			},
		},
		"uuid generation fails": {
			seed:       func(r *testutil.IdentityRepository) { r.Seed(testutil.KnownIdentity()) },
			body:       `{"email": "known@example.com", "password": "correct-horse-battery-staple"}`,
			uuidErr:    errUUID,
			wantStatus: http.StatusInternalServerError,
			wantBody: `{
				"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md",
				"title": "Server Error",
				"status": 500,
				"code": "500-01",
				"detail": "The server encountered an unexpected internal error",
				"instance": "/iam/tokens"
			}`,
			wantLog: &slogmem.RecordQuery{
				Level:   slog.LevelWarn,
				Message: "Failed to generate uuid for jwt token",
				Attrs:   map[string]slog.Value{"error": slog.AnyValue(errUUID)},
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

			gen := testutil.NewUUIDV4Generator(uuid.MustParse("44444444-4444-4444-4444-444444444444"))
			gen.Err = tc.uuidErr

			server, records := newServer(t, repo, gen)

			req := httptest.NewRequest(http.MethodPost, "/iam/tokens", bytes.NewBufferString(tc.body))
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
