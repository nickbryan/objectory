//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickbryan/objectory/api/internal/testutil"
)

func TestLogin_ReturnsTokenForValidCredentials(t *testing.T) {
	t.Parallel()

	server, repo, _ := newWiredServer(t)

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
		"password": "wrong-password"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/iam/tokens", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	testutil.ProblemResponse(t, rec, http.StatusUnauthorized, `{
		"type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/unauthorized.md",
		"title": "Unauthorized",
		"status": 401,
		"code": "401-01",
		"detail": "You must be authenticated to POST this resource",
		"instance": "/iam/tokens"
	}`)
}

func TestMe_ReturnsCurrentIdentityForValidToken(t *testing.T) {
	t.Parallel()

	server, repo, _ := newWiredServer(t)

	if err := repo.Create(context.Background(), testutil.KnownIdentity()); err != nil {
		t.Fatalf("seed: %v", err)
	}

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
}
