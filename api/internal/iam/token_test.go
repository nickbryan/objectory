package iam_test

import (
	"bytes"
	"encoding/json"
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
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
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
