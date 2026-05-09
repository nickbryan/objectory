// Package storage encapsulates the database interaction.
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
)

// IdentityRepository encapsulates the persistence logic for iam.Identity
// records.
type IdentityRepository struct {
	queries *postgres.Queries
	now     func() time.Time
}

// NewIdentityRepository creates a new IdentityRepository that is connected to a
// postgres database.
func NewIdentityRepository(queries *postgres.Queries, now func() time.Time) *IdentityRepository {
	return &IdentityRepository{queries: queries, now: now}
}

// Create writes a new iam.Identity to the database.
func (ir *IdentityRepository) Create(ctx context.Context, identity iam.Identity) error {
	now := pgtype.Timestamp{
		Time:  ir.now(),
		Valid: true,
	}

	err := ir.queries.CreateIdentity(ctx, postgres.CreateIdentityParams{
		ID: pgtype.UUID{
			Bytes: identity.ID,
			Valid: true,
		},
		Email:     identity.Email,
		Password:  identity.Password.String(),
		Name:      identity.Name,
		CreatedAt: now,
		UpdatedAt: now,
	})

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return iam.ErrDuplicateIdentity
	}

	if err != nil {
		return fmt.Errorf("running database query: %w", err)
	}

	return nil
}

// Find finds an iam.Identity matching the given id string.
func (ir *IdentityRepository) Find(ctx context.Context, id uuid.UUID) (*iam.Identity, error) {
	identity, err := ir.queries.Identity(ctx, pgtype.UUID{Bytes: id, Valid: true})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, iam.ErrIdentityNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("running database query: %w", err)
	}

	return &iam.Identity{
		ID:       identity.ID.Bytes,
		Name:     identity.Name,
		Email:    identity.Email,
		Password: iam.NewHashedPassword([]byte(identity.Password)),
	}, nil
}

// FindByEmail finds an iam.Identity matching the given email string.
func (ir *IdentityRepository) FindByEmail(ctx context.Context, email string) (*iam.Identity, error) {
	identity, err := ir.queries.IdentityByEmail(ctx, email)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, iam.ErrIdentityNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("running database query: %w", err)
	}

	return &iam.Identity{
		ID:       identity.ID.Bytes,
		Name:     identity.Name,
		Email:    identity.Email,
		Password: iam.NewHashedPassword([]byte(identity.Password)),
	}, nil
}
