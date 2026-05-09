package iam

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang-jwt/jwt/v5/request"
	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

type claims struct {
	jwt.RegisteredClaims

	UUID uuid.UUID `json:"uuid"`
}

type currentIdentityCtxKey struct{}

// CurrentIdentityFromContext will fetch the currently authenticated Identity
// identifier from the given context.
func CurrentIdentityFromContext(ctx context.Context) (uuid.UUID, bool) {
	identity, ok := ctx.Value(currentIdentityCtxKey{}).(uuid.UUID)
	return identity, ok
}

// NewJWTGuard returns a GuardFunc that validates a Bearer JWT from the
// Authorization header, signed with jwtKey. On success it stores the
// identity UUID in the request context; on failure it returns a 403
// Forbidden problem response.
func NewJWTGuard(logger *slog.Logger, jwtKey string) httputil.GuardFunc {
	return func(r *http.Request) (*http.Request, error) {
		token, err := request.ParseFromRequest(r, request.BearerExtractor{}, func(_ *jwt.Token) (any, error) {
			return []byte(jwtKey), nil
		}, request.WithClaims(&claims{}))
		if err != nil {
			logger.InfoContext(r.Context(), "Authentication denied invalid jwt", slog.Any("error", err))
			return nil, problem.Forbidden(r)
		}

		clms, ok := token.Claims.(*claims)
		if !ok {
			logger.WarnContext(r.Context(), "Authentication failed jwt claims invalid")

			return nil, problem.ServerError(r)
		}

		return r.WithContext(context.WithValue(r.Context(), currentIdentityCtxKey{}, clms.UUID)), nil
	}
}
