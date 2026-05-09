// Package api provides the HTTP client for the IAM API.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nickbryan/httputil"
)

const (
	// TypeResourceExists is the problem type URI returned by the API when a
	// resource already exists. It matches the type field in the server's RFC
	// 9457 problem responses.
	TypeResourceExists = "https://github.com/nickbryan/httputil/blob/main/docs/problems/resource-exists.md"
)

// ErrEmailAlreadyRegistered is returned by CreateIdentity when the API
// responds with a resource-exists problem, indicating the email is in use.
var ErrEmailAlreadyRegistered = errors.New("email already registered")

// IAMClient wraps the httputil.Client to communicate with the IAM API.
type IAMClient struct {
	client *httputil.Client
}

// NewIAMClient creates a new IAMClient that communicates with the API at the given base URL.
func NewIAMClient(logger *slog.Logger, baseURL string) *IAMClient {
	return &IAMClient{
		client: httputil.NewClient(
			logger,
			httputil.WithClientBasePath(baseURL),
			httputil.WithClientCodec(httputil.NewJSONClientCodec()),
		),
	}
}

// CreateIdentityRequest holds the data for creating a new identity.
type CreateIdentityRequest struct {
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

// CreateIdentityResponse holds the response from a successful identity creation.
type CreateIdentityResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// CreateIdentity sends a request to the API to create a new identity.
func (c *IAMClient) CreateIdentity(ctx context.Context, req CreateIdentityRequest) (*CreateIdentityResponse, error) {
	result, err := httputil.Post[CreateIdentityResponse](ctx, c.client, "/iam/identities", req)
	if err != nil {
		if pe, ok := errors.AsType[*httputil.ProblemResponseError](err); ok && pe.Problem.Type == TypeResourceExists {
			return nil, ErrEmailAlreadyRegistered
		}

		return nil, fmt.Errorf("creating identity: %w", err)
	}

	return &result.Data, nil
}

// CreateTokenRequest holds the data for creating a new authentication token.
type CreateTokenRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateTokenResponse holds the response from a successful token creation.
type CreateTokenResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

// CreateToken sends a request to the API to create a new authentication token.
func (c *IAMClient) CreateToken(ctx context.Context, req CreateTokenRequest) (*CreateTokenResponse, error) {
	result, err := httputil.Post[CreateTokenResponse](ctx, c.client, "/iam/tokens", req)
	if err != nil {
		return nil, fmt.Errorf("creating token: %w", err)
	}

	return &result.Data, nil
}

// IdentityResponse holds the response from an identity lookup.
type IdentityResponse struct {
	Data struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"data"`
}

// GetMe fetches the current authenticated identity using the given auth token.
func (c *IAMClient) GetMe(ctx context.Context, token string) (*IdentityResponse, error) {
	result, err := httputil.Get[IdentityResponse](ctx, c.client, "/iam/identities/me",
		httputil.WithRequestHeader("Authorization", "Bearer "+token),
	)
	if err != nil {
		return nil, fmt.Errorf("getting identity: %w", err)
	}

	return &result.Data, nil
}
