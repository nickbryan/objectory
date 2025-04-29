package iam

import (
	"context"
	"log/slog"
	"net/http"
	"os"

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

func NewJWTGuard(logger *slog.Logger) httputil.GuardFunc {
	return func(r *http.Request) (*http.Request, error) {
		token, err := request.ParseFromRequest(r, request.BearerExtractor{}, func(_ *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_KEY")), nil
		}, request.WithClaims(&claims{})) //nolint:exhaustruct
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
