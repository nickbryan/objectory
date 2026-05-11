// Package uuidgen provides a v4 UUID generator backed by github.com/google/uuid.
// It exists as a small, testable wrapper so that production wiring does not
// inline uuid.NewRandom() calls and so that the package boundary makes the
// dependency explicit at the seam.
package uuidgen

import (
	"fmt"

	"github.com/google/uuid"
)

// Generator produces cryptographically random v4 UUIDs. It satisfies
// iam.UUIDV4Generator. Its zero value is ready to use.
type Generator struct{}

// New returns a Generator.
func New() Generator {
	return Generator{}
}

// GenerateUUIDV4 returns a fresh v4 UUID as raw bytes.
func (Generator) GenerateUUIDV4() ([16]byte, error) {
	next, err := uuid.NewRandom()
	if err != nil {
		return [16]byte{}, fmt.Errorf("creating new random uuid: %w", err)
	}

	return next, nil
}
