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
