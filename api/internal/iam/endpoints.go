package iam

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
)

type IdentityRepository interface {
	Create(ctx context.Context, identity Identity) error
	Find(ctx context.Context, id uuid.UUID) (*Identity, error)
	FindByEmail(ctx context.Context, email string) (*Identity, error)
}

type TokenRepository interface {
	Create(id string) (string, error)
}

// UUIDV4Generator generates a new UUID V4 as an array of bytes.
type UUIDV4Generator interface {
	GenerateUUIDV4() ([16]byte, error)
}

func Endpoints(
	logger *slog.Logger,
	uuidV4Generator UUIDV4Generator,
	identityRepository IdentityRepository,
) httputil.EndpointGroup {
	endpoints := httputil.EndpointGroup{
		{
			Path:    "/identities",
			Method:  http.MethodPost,
			Handler: identityCreateHandler(logger, uuidV4Generator, identityRepository),
		},
		{
			Path:    "/tokens",
			Method:  http.MethodPost,
			Handler: tokenCreateHandler(logger, identityRepository),
		},
	}

	authEndpoints := httputil.EndpointGroup{
		{
			Path:    "/identities/{id}",
			Method:  http.MethodGet,
			Handler: identityLookupHandler(logger, identityRepository),
		},
	}.WithGuard(NewJWTGuard(logger))

	return append(endpoints, authEndpoints...).WithPrefix("/iam")
}
