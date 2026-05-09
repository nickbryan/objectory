package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/iam"
)

// IdentityRepository is an in-memory fake of iam.IdentityRepository with
// optional error injection. Default behaviour mirrors the real repository:
// duplicate emails return iam.ErrDuplicateIdentity, missing rows return
// iam.ErrIdentityNotFound. Methods are safe for concurrent use.
type IdentityRepository struct {
	mu      sync.Mutex
	byID    map[uuid.UUID]iam.Identity
	byEmail map[string]uuid.UUID

	// CreateErr, FindErr, FindByEmailErr, when non-nil, cause every call to
	// that method to return the given error before touching the in-memory
	// store. Clear the field to resume normal behaviour.
	CreateErr      error
	FindErr        error
	FindByEmailErr error
}

// NewIdentityRepository returns an empty fake.
func NewIdentityRepository() *IdentityRepository {
	return &IdentityRepository{
		byID:    make(map[uuid.UUID]iam.Identity),
		byEmail: make(map[string]uuid.UUID),
	}
}

// Seed inserts an identity directly, bypassing duplicate checks. Use this
// from tests to set up preconditions.
func (r *IdentityRepository) Seed(identity iam.Identity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[identity.ID] = identity
	r.byEmail[identity.Email] = identity.ID
}

func (r *IdentityRepository) Create(_ context.Context, identity iam.Identity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CreateErr != nil {
		return r.CreateErr
	}
	if _, exists := r.byEmail[identity.Email]; exists {
		return iam.ErrDuplicateIdentity
	}
	r.byID[identity.ID] = identity
	r.byEmail[identity.Email] = identity.ID
	return nil
}

func (r *IdentityRepository) Find(_ context.Context, id uuid.UUID) (*iam.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FindErr != nil {
		return nil, r.FindErr
	}
	identity, ok := r.byID[id]
	if !ok {
		return nil, iam.ErrIdentityNotFound
	}
	return &identity, nil
}

func (r *IdentityRepository) FindByEmail(_ context.Context, email string) (*iam.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FindByEmailErr != nil {
		return nil, r.FindByEmailErr
	}
	id, ok := r.byEmail[email]
	if !ok {
		return nil, iam.ErrIdentityNotFound
	}
	identity := r.byID[id]
	return &identity, nil
}

// UUIDV4Generator is a fake of iam.UUIDV4Generator that returns pre-seeded
// UUIDs in order. It supports error injection via Err.
type UUIDV4Generator struct {
	mu    sync.Mutex
	UUIDs []uuid.UUID
	idx   int

	// Err, when non-nil, causes every call to GenerateUUIDV4 to return this
	// error. Clear it to resume returning seeded UUIDs.
	Err error
}

// NewUUIDV4Generator returns a generator pre-seeded with the given UUIDs.
func NewUUIDV4Generator(uuids ...uuid.UUID) *UUIDV4Generator {
	return &UUIDV4Generator{UUIDs: uuids}
}

func (g *UUIDV4Generator) GenerateUUIDV4() ([16]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Err != nil {
		return [16]byte{}, g.Err
	}
	if g.idx >= len(g.UUIDs) {
		return [16]byte{}, errors.New("testutil.UUIDV4Generator: ran out of pre-seeded UUIDs")
	}
	next := g.UUIDs[g.idx]
	g.idx++
	return [16]byte(next), nil
}
