package iam_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/objectory/api/internal/testutil"
)

// handlerErrorCase parameterises a table-driven test of a JSON handler that
// produces a problem-details response on error and optionally writes a log
// record. Used by the *_Errors tests in this package.
type handlerErrorCase struct {
	seed       func(*testutil.IdentityRepository)
	body       string
	repoErr    func(*testutil.IdentityRepository)
	uuidErr    error
	wantStatus int
	wantBody   string
	wantLog    *slogmem.RecordQuery
}

// runHandlerErrorCases drives a map of handlerErrorCase against POST path,
// using fallbackUUID as the next UUID returned by the generator unless a
// case sets uuidErr.
func runHandlerErrorCases(
	t *testing.T,
	path string,
	fallbackUUID uuid.UUID,
	cases map[string]handlerErrorCase,
) {
	t.Helper()

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

			gen := testutil.NewUUIDV4Generator(fallbackUUID)
			gen.Err = tc.uuidErr

			server, records := newServer(t, repo, gen)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewBufferString(tc.body))
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
