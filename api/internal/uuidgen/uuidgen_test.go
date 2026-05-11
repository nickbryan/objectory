package uuidgen_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/nickbryan/objectory/api/internal/uuidgen"
)

func TestNew_GenerateUUIDV4_ReturnsValidV4(t *testing.T) {
	t.Parallel()

	gen := uuidgen.New()

	bytes, err := gen.GenerateUUIDV4()
	if err != nil {
		t.Fatalf("GenerateUUIDV4: %v", err)
	}

	got := uuid.UUID(bytes)
	if got.Version() != 4 {
		t.Errorf("version: got %d, want 4", got.Version())
	}

	if got.Variant() != uuid.RFC4122 {
		t.Errorf("variant: got %d, want %d (RFC 4122)", got.Variant(), uuid.RFC4122)
	}

	if got == uuid.Nil {
		t.Error("got nil uuid")
	}
}

func TestNew_GenerateUUIDV4_ReturnsUnique(t *testing.T) {
	t.Parallel()

	gen := uuidgen.New()
	seen := make(map[uuid.UUID]struct{}, 100)

	for range 100 {
		b, err := gen.GenerateUUIDV4()
		if err != nil {
			t.Fatalf("GenerateUUIDV4: %v", err)
		}

		id := uuid.UUID(b)
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate uuid: %s", id)
		}

		seen[id] = struct{}{}
	}
}
