// Package testutil provides shared helpers for the api module's test suites:
// fakes for iam collaborators, response assertions, fixtures, and Postgres
// container/database lifecycle.
package testutil

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nickbryan/objectory/api/internal/iam"
)

// JWTKey is a fixed 32-byte signing key shared by every test that touches
// JWTs. HS256 requires keys of at least 32 bytes (256 bits).
const JWTKey = "test-jwt-key-32-bytes-padding!!!"

// KnownPassword is the plaintext that matches the hash in KnownIdentity.
const KnownPassword = "correct-horse-battery-staple"

// FixedTime is the canonical instant used by tests that inject a clock.
var FixedTime = time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

// Clock returns a now() function that always returns FixedTime, suitable for
// passing to iam.Endpoints or storage.NewIdentityRepository in tests.
func Clock() func() time.Time {
	return func() time.Time { return FixedTime }
}

// KnownIdentityID is the stable UUID used for the fixture identity.
var KnownIdentityID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

var knownIdentityHash []byte

func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte(KnownPassword), bcrypt.MinCost)
	if err != nil {
		panic("testutil: precomputing fixture password hash: " + err.Error())
	}
	knownIdentityHash = hash
}

// KnownIdentity returns a fixture iam.Identity with a stable ID, name, email
// and a precomputed bcrypt hash (cost = bcrypt.MinCost) for the password
// KnownPassword.
func KnownIdentity() iam.Identity {
	return iam.Identity{
		ID:       KnownIdentityID,
		Name:     "Known Test User",
		Email:    "known@example.com",
		Password: iam.NewHashedPassword(knownIdentityHash),
	}
}
