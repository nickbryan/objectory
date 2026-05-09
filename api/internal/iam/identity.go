// Package iam implements the Identity and Access Management domain,
// including identity creation, authentication, and JWT-based authorization.
package iam

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

var (
	// ErrDuplicateIdentity is returned when attempting to create an Identity that
	// already exists.
	ErrDuplicateIdentity = errors.New("an identity must be unique")

	// ErrIdentityNotFound is used when an entity cannot be found with the given
	// identifier.
	ErrIdentityNotFound = errors.New("identity not found")
)

// envelope is the API's standard JSON response shape. Adding optional
// top-level fields (e.g. Meta, Links) is a matter of adding `omitempty`
// fields here; existing call sites stay unchanged.
type envelope struct {
	Data any `json:"data"`
}

// An Identity represents the information required to
// identify a user of the application.
type Identity struct {
	ID       uuid.UUID
	Name     string
	Email    string
	Password PasswordHash
}

// PasswordHash is used to check the validity of a password on an Identity.
type PasswordHash struct {
	hash []byte
}

// NewPasswordHash creates a new PasswordHash from a password string.
// Internally, a bcrypt hash is calculated and stored for future comparison.
func NewPasswordHash(password string) (PasswordHash, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return PasswordHash{}, fmt.Errorf("generating hash from password: %w", err)
	}

	return PasswordHash{hash: hash}, nil
}

// NewHashedPassword creates a PasswordHash from an already hashed password.
func NewHashedPassword(hash []byte) PasswordHash {
	return PasswordHash{hash: hash}
}

// Matches returns true if the hashed version of the given password string
// matches the internal PasswordHash hash value.
func (h PasswordHash) Matches(password string) bool {
	return bcrypt.CompareHashAndPassword(h.hash, []byte(password)) == nil
}

// String returns the string version of the PasswordHash.
func (h PasswordHash) String() string {
	return string(h.hash)
}

func identityMeHandler(logger *slog.Logger, identities IdentityRepository) http.Handler {
	type response struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	return httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
		currentIdentity, ok := CurrentIdentityFromContext(r.Context())
		if !ok {
			logger.ErrorContext(r.Context(), "Failed to get current identity from context")
			return nil, problem.ServerError(r.Request)
		}

		identity, err := identities.Find(r.Context(), currentIdentity)
		if errors.Is(err, ErrIdentityNotFound) {
			return nil, problem.NotFound(r.Request)
		} else if err != nil {
			logger.ErrorContext(r.Context(), "Failed to find Identity", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		return httputil.OK(envelope{Data: response{
			ID:    identity.ID.String(),
			Name:  identity.Name,
			Email: identity.Email,
		}})
	})
}

func identityCreateHandler(logger *slog.Logger, uuidGenerator UUIDV4Generator, identities IdentityRepository) http.Handler {
	type (
		request struct {
			Name                 string `json:"name"                 validate:"required,max=64"`
			Email                string `json:"email"                validate:"required,email"`
			Password             string `json:"password"             validate:"required,min=8,max=64,eqfield=PasswordConfirmation"`
			PasswordConfirmation string `json:"passwordConfirmation" validate:"required"`
		}

		response struct {
			ID string `json:"id"`
		}
	)

	return httputil.NewHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		uniqueID, err := uuidGenerator.GenerateUUIDV4()
		if err != nil {
			logger.ErrorContext(r.Context(), "Failed to generate uuid for new Identity", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		passwordHash, err := NewPasswordHash(r.Data.Password)
		if err != nil {
			logger.ErrorContext(r.Context(), "Failed to create password hash for new Identity", slog.Any("error", err))
			return nil, problem.ServerError(r.Request)
		}

		identity := Identity{
			ID:       uniqueID,
			Name:     r.Data.Name,
			Email:    r.Data.Email,
			Password: passwordHash,
		}

		if err = identities.Create(r.Context(), identity); err != nil {
			if errors.Is(err, ErrDuplicateIdentity) {
				return nil, problem.ResourceExists(r.Request)
			}

			logger.ErrorContext(r.Context(), "Failed to create new Identity", slog.Any("error", err))

			return nil, problem.ServerError(r.Request)
		}

		logger.InfoContext(r.Context(), "Successfully created new Identity", slog.String("id", identity.ID.String()))

		return httputil.Created(envelope{Data: response{ID: identity.ID.String()}})
	})
}
