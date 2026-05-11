package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/iam"
)

// errOutOfUUIDs is returned by UUIDV4Generator when its pre-seeded slice is
// exhausted. Tests that hit it have either under-seeded the fake or called
// the generator more times than expected.
var errOutOfUUIDs = errors.New("testutil.UUIDV4Generator: ran out of pre-seeded UUIDs")

// IdentityRepository is an in-memory fake of iam.IdentityRepository with
// optional error injection. Default behavior mirrors the real repository:
// duplicate emails return iam.ErrDuplicateIdentity, missing rows return
// iam.ErrIdentityNotFound. Methods are safe for concurrent use.
type IdentityRepository struct {
	mu      sync.Mutex
	byID    map[uuid.UUID]iam.Identity
	byEmail map[string]uuid.UUID

	// CreateErr, FindErr, FindByEmailErr, when non-nil, cause every call to
	// that method to return the given error before touching the in-memory
	// store. Clear the field to resume normal behavior.
	CreateErr      error
	FindErr        error
	FindByEmailErr error
}

// NewIdentityRepository returns an empty fake.
func NewIdentityRepository() *IdentityRepository {
	return &IdentityRepository{
		mu:             sync.Mutex{},
		byID:           make(map[uuid.UUID]iam.Identity),
		byEmail:        make(map[string]uuid.UUID),
		CreateErr:      nil,
		FindErr:        nil,
		FindByEmailErr: nil,
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

// Create stores identity, returning iam.ErrDuplicateIdentity if an identity
// with the same email already exists. If CreateErr is set, returns it
// without touching the store.
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

// Find returns the identity with the given id, or iam.ErrIdentityNotFound
// when missing. If FindErr is set, returns it without touching the store.
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

// FindByEmail returns the identity with the given email, or
// iam.ErrIdentityNotFound when missing. If FindByEmailErr is set, returns it
// without touching the store.
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
	return &UUIDV4Generator{
		mu:    sync.Mutex{},
		UUIDs: uuids,
		idx:   0,
		Err:   nil,
	}
}

// GenerateUUIDV4 returns the next pre-seeded UUID, or Err when set.
// Returns errOutOfUUIDs once the seeded slice is exhausted.
func (g *UUIDV4Generator) GenerateUUIDV4() ([16]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Err != nil {
		return [16]byte{}, g.Err
	}

	if g.idx >= len(g.UUIDs) {
		return [16]byte{}, errOutOfUUIDs
	}

	next := g.UUIDs[g.idx]
	g.idx++

	return [16]byte(next), nil
}
