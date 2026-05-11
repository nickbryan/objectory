package iam_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang-jwt/jwt/v5/request"
	"github.com/google/uuid"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func TestCurrentIdentityFromContext(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		ctx    context.Context
		wantID uuid.UUID
		wantOk bool
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
		header     string
		wantOK     bool
		wantErrLog error // sentinel the auth-denied log's "error" attr must wrap
	}{
		"valid token populates context": {
			header: "Bearer " + signTestJWT(t, testutil.KnownIdentityID),
			wantOK: true,
		},
		"missing authorization header errors": {
			header:     "",
			wantOK:     false,
			wantErrLog: request.ErrNoTokenInRequest,
		},
		"wrong signing key errors": {
			header:     "Bearer " + signJWTWithKey(t, "00000000000000000000000000000000-other"),
			wantOK:     false,
			wantErrLog: jwt.ErrTokenSignatureInvalid,
		},
		"expired token errors": {
			header:     "Bearer " + signExpiredJWT(t),
			wantOK:     false,
			wantErrLog: jwt.ErrTokenExpired,
		},
		"malformed token errors": {
			header:     "Bearer not.a.jwt",
			wantOK:     false,
			wantErrLog: jwt.ErrTokenMalformed,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			logger, records := slogutil.NewInMemoryLogger(slog.LevelDebug)
			guard := iam.NewJWTGuard(logger, testutil.JWTKey)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/protected", nil)
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

				if !records.IsEmpty() {
					t.Errorf("expected no logs on success path; got %d", records.Len())
				}

				return
			}

			if err == nil {
				t.Fatalf("expected error, got nil; passed = %v", passed)
			}

			if passed != nil {
				t.Errorf("expected nil request on error path, got %v", passed)
			}

			if tc.wantErrLog != nil {
				assertAuthDeniedLog(t, records, tc.wantErrLog)
			}
		})
	}
}

// assertAuthDeniedLog asserts that records contains an INFO log with message
// "Authentication denied invalid jwt" whose "error" attr wraps target (per
// errors.Is). The walk goes through AsSliceOfNestedKeyValuePairs because
// slogmem.RecordQuery compares attr values by equality, which doesn't match
// wrapped errors returned by the jwt parser.
func assertAuthDeniedLog(t *testing.T, records *slogmem.LoggedRecords, target error) {
	t.Helper()

	for _, r := range records.AsSliceOfNestedKeyValuePairs() {
		if lvl, _ := r["level"].(slog.Level); lvl != slog.LevelInfo {
			continue
		}

		if msg, _ := r["msg"].(string); msg != "Authentication denied invalid jwt" {
			continue
		}

		e, ok := r["error"].(error)
		if !ok {
			continue
		}

		if errors.Is(e, target) {
			return
		}
	}

	t.Errorf("no auth-denied log with error wrapping %v\nrecords: %+v", target, records.AsSliceOfNestedKeyValuePairs())
}

func signJWTWithKey(t *testing.T, key string) string {
	t.Helper()

	type testClaims struct {
		jwt.RegisteredClaims

		UUID uuid.UUID `json:"uuid"`
	}

	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
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

	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
		},
		UUID: testutil.KnownIdentityID,
	})

	signed, err := tok.SignedString([]byte(testutil.JWTKey))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	return signed
}
