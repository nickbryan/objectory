package iam

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
)

// IdentityRepository defines the persistence operations for Identity records.
type IdentityRepository interface {
	Create(ctx context.Context, identity Identity) error
	Find(ctx context.Context, id uuid.UUID) (*Identity, error)
	FindByEmail(ctx context.Context, email string) (*Identity, error)
}

// UUIDV4Generator generates a new UUID V4 as an array of bytes.
type UUIDV4Generator interface {
	GenerateUUIDV4() ([16]byte, error)
}

// Endpoints returns the EndpointGroup for the IAM API. The jwtKey is used to
// sign and verify JWTs; it must not be empty (validated by the caller).
// passwordCost is forwarded to NewPasswordHash; main.go passes
// bcrypt.DefaultCost. now is the clock used for JWT iat/exp; main.go passes
// time.Now.
func Endpoints(
	logger *slog.Logger,
	uuidV4Generator UUIDV4Generator,
	identityRepository IdentityRepository,
	jwtKey string,
	passwordCost int,
	now func() time.Time,
) httputil.EndpointGroup {
	publicEndpoints := httputil.EndpointGroup{
		{
			Path:    "/identities",
			Method:  http.MethodPost,
			Handler: identityCreateHandler(logger, uuidV4Generator, identityRepository, passwordCost),
		},
		{
			Path:    "/tokens",
			Method:  http.MethodPost,
			Handler: tokenCreateHandler(logger, identityRepository, uuidV4Generator, jwtKey, now),
		},
	}

	authEndpoints := httputil.EndpointGroup{
		{
			Path:    "/identities/me",
			Method:  http.MethodGet,
			Handler: identityMeHandler(logger, identityRepository),
		},
	}.WithGuard(NewJWTGuard(logger, jwtKey))

	endpoints := make(httputil.EndpointGroup, 0, len(publicEndpoints)+len(authEndpoints))
	endpoints = append(endpoints, publicEndpoints...)
	endpoints = append(endpoints, authEndpoints...)

	return endpoints.WithPrefix("/iam")
}

// envelope is the API's standard JSON response shape. Adding optional
// top-level fields (e.g. Meta, Links) is a matter of adding `omitempty`
// fields here; existing call sites stay unchanged.
type envelope struct {
	Data any `json:"data"`
}
