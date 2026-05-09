package iam

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

func tokenCreateHandler(logger *slog.Logger, identities IdentityRepository, uuidGenerator UUIDV4Generator, jwtKey string, now func() time.Time) http.Handler {
	const oneDay = 24 * time.Hour

	// Task 7: replace uuid.NewRandom() below with uuidGenerator.GenerateUUIDV4(),
	// and the two time.Now() calls in the JWT claims with now(). The discard
	// assignments suppress unused-parameter lints until that wiring lands.
	_ = uuidGenerator
	_ = now

	type (
		request struct {
			Email    string `json:"email"    validate:"required,email"`
			Password string `json:"password" validate:"required,min=8,max=64"`
		}

		response struct {
			Token string `json:"token"`
		}
	)

	return httputil.NewHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		identity, err := identities.FindByEmail(r.Context(), r.Data.Email)
		if errors.Is(err, ErrIdentityNotFound) {
			return nil, problem.Unauthorized(r.Request)
		} else if err != nil {
			logger.WarnContext(r.Context(), "Failed to find identity by email when creating new token", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		if !identity.Password.Matches(r.Data.Password) {
			return nil, problem.Unauthorized(r.Request)
		}

		jwtID, err := uuid.NewRandom()
		if err != nil {
			logger.WarnContext(r.Context(), "Failed to generate uuid for jwt token", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "objectory",
				Subject:   "authentication",
				Audience:  jwt.ClaimStrings{"objectory"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(oneDay)),
				NotBefore: nil,
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ID:        jwtID.String(),
			},
			UUID: identity.ID,
		})

		tokenString, err := token.SignedString([]byte(jwtKey))
		if err != nil {
			logger.WarnContext(r.Context(), "Failed to create signed token string", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		return httputil.Created(envelope{Data: response{Token: tokenString}})
	})
}
