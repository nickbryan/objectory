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
		header string
		wantOK bool
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
