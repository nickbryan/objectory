// Package auth provides session/auth-token middleware and form handlers for
// the web app.
package auth

import (
	"context"
	"net/http"
)

type tokenCtxKey struct{}

// TokenFromContext returns the auth token stored in the context by
// RequireAuth. This should only be called from handlers behind RequireAuth;
// callers that hit an unauthenticated context get an empty string.
func TokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(tokenCtxKey{}).(string)
	return token
}

// RequireAuth is middleware that checks for an auth_token cookie. If present,
// the token value is stored in the request context. If absent, the request is
// redirected to "/".
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth_token")
		if err != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), tokenCtxKey{}, cookie.Value)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
