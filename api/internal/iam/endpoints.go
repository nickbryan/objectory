package iam

import (
	"context"
	"log/slog"
	"net/http"

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
func Endpoints(
	logger *slog.Logger,
	uuidV4Generator UUIDV4Generator,
	identityRepository IdentityRepository,
	jwtKey string,
) httputil.EndpointGroup {
	publicEndpoints := httputil.EndpointGroup{
		{
			Path:    "/identities",
			Method:  http.MethodPost,
			Handler: identityCreateHandler(logger, uuidV4Generator, identityRepository),
		},
		{
			Path:    "/tokens",
			Method:  http.MethodPost,
			Handler: tokenCreateHandler(logger, identityRepository, jwtKey),
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
