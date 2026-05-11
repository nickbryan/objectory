// Package testutil provides shared helpers for the api module's test suites:
// fakes for iam collaborators, response assertions, fixtures, and Postgres
// container/database lifecycle.
package testutil

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/objectory/api/internal/iam"
)

const (
	// JWTKey is a fixed 32-byte signing key shared by every test that touches
	// JWTs. HS256 requires keys of at least 32 bytes (256 bits).
	JWTKey = "test-jwt-key-32-bytes-padding!!!"

	// KnownPassword is the plaintext that matches the hash in KnownIdentity.
	KnownPassword = "correct-horse-battery-staple"
)

// Shared read-only fixtures. They are package-level vars because they are
// referenced as values by many tests across packages; a function-based form
// would force every call site to add empty parens with no behavioral gain.
//
//nolint:gochecknoglobals // shared read-only test fixtures
var (
	// FixedTime is the canonical instant used by tests that inject a clock.
	FixedTime = time.Date(2026, time.May, 9, 12, 0, 0, 0, time.UTC)

	// KnownIdentityID is the stable UUID used for the fixture identity.
	KnownIdentityID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// knownIdentityHash lazily computes the bcrypt hash for KnownPassword.
	// bcrypt with MinCost is cheap but not free; compute once per process.
	knownIdentityHash = sync.OnceValue(func() []byte {
		hash, err := bcrypt.GenerateFromPassword([]byte(KnownPassword), bcrypt.MinCost)
		if err != nil {
			panic("testutil: precomputing fixture password hash: " + err.Error())
		}

		return hash
	})
)

// Clock returns a now() function that always returns FixedTime, suitable for
// passing to iam.Endpoints or storage.NewIdentityRepository in tests.
func Clock() func() time.Time {
	return func() time.Time { return FixedTime }
}

// KnownIdentity returns a fixture iam.Identity with a stable ID, name, email
// and a precomputed bcrypt hash (cost = bcrypt.MinCost) for the password
// KnownPassword.
func KnownIdentity() iam.Identity {
	return iam.Identity{
		ID:       KnownIdentityID,
		Name:     "Known Test User",
		Email:    "known@example.com",
		Password: iam.NewHashedPassword(knownIdentityHash()),
	}
}
