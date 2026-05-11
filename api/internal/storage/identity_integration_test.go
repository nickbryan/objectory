//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nickbryan/objectory/api/internal/iam"
	"github.com/nickbryan/objectory/api/internal/storage"
	"github.com/nickbryan/objectory/api/internal/storage/postgres"
	"github.com/nickbryan/objectory/api/internal/testutil"
)

func TestIdentityRepository_Create(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed    func(*testing.T, *storage.IdentityRepository)
		input   iam.Identity
		wantErr error
	}{
		"creates a new identity": {
			input: iam.Identity{
				ID:       testutil.KnownIdentityID,
				Name:     "Alice",
				Email:    "alice@example.com",
				Password: iam.NewHashedPassword([]byte("hashed")),
			},
		},
		"duplicate email returns ErrDuplicateIdentity": {
			seed: func(t *testing.T, r *storage.IdentityRepository) {
				t.Helper()

				err := r.Create(context.Background(), iam.Identity{
					ID:       testutil.KnownIdentityID,
					Name:     "First",
					Email:    "dup@example.com",
					Password: iam.NewHashedPassword([]byte("hashed")),
				})
				if err != nil {
					t.Fatalf("seed: %v", err)
				}
			},
			input: iam.Identity{
				ID:       uuid.MustParse("99999999-9999-9999-9999-999999999999"),
				Name:     "Second",
				Email:    "dup@example.com",
				Password: iam.NewHashedPassword([]byte("hashed")),
			},
			wantErr: iam.ErrDuplicateIdentity,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := testutil.NewTestDB(t)
			repo := storage.NewIdentityRepository(postgres.New(pool), testutil.Clock())

			if tc.seed != nil {
				tc.seed(t, repo)
			}

			err := repo.Create(context.Background(), tc.input)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}

			if tc.wantErr != nil {
				return
			}

			var (
				gotID        pgtype.UUID
				gotEmail     string
				gotName      string
				gotPassword  string
				gotCreatedAt pgtype.Timestamp
				gotUpdatedAt pgtype.Timestamp
			)

			err = pool.QueryRow(context.Background(),
				`SELECT id, email, name, password, created_at, updated_at FROM iam.identities WHERE id = $1`,
				pgtype.UUID{Bytes: tc.input.ID, Valid: true},
			).Scan(&gotID, &gotEmail, &gotName, &gotPassword, &gotCreatedAt, &gotUpdatedAt)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}

			if diff := cmp.Diff(tc.input.Email, gotEmail); diff != "" {
				t.Errorf("email mismatch:\n%s", diff)
			}

			if diff := cmp.Diff(tc.input.Name, gotName); diff != "" {
				t.Errorf("name mismatch:\n%s", diff)
			}

			if !gotCreatedAt.Time.Equal(testutil.FixedTime) {
				t.Errorf("created_at: got %v, want %v", gotCreatedAt.Time, testutil.FixedTime)
			}

			if !gotUpdatedAt.Time.Equal(testutil.FixedTime) {
				t.Errorf("updated_at: got %v, want %v", gotUpdatedAt.Time, testutil.FixedTime)
			}
		})
	}
}

func TestIdentityRepository_Find(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed    func(*testing.T, *storage.IdentityRepository)
		findID  uuid.UUID
		wantErr error
		wantOK  bool
	}{
		"finds an existing identity": {
			seed: func(t *testing.T, r *storage.IdentityRepository) {
				t.Helper()

				if err := r.Create(context.Background(), testutil.KnownIdentity()); err != nil {
					t.Fatalf("seed: %v", err)
				}
			},
			findID: testutil.KnownIdentityID,
			wantOK: true,
		},
		"returns ErrIdentityNotFound when missing": {
			findID:  testutil.KnownIdentityID,
			wantErr: iam.ErrIdentityNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := testutil.NewTestDB(t)
			repo := storage.NewIdentityRepository(postgres.New(pool), testutil.Clock())

			if tc.seed != nil {
				tc.seed(t, repo)
			}

			got, err := repo.Find(context.Background(), tc.findID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}

			if tc.wantOK {
				if got == nil {
					t.Fatal("expected identity, got nil")
				}

				if got.Email != "known@example.com" {
					t.Errorf("email: got %s, want known@example.com", got.Email)
				}
			}
		})
	}
}

func TestIdentityRepository_FindByEmail(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		seed    func(*testing.T, *storage.IdentityRepository)
		email   string
		wantErr error
		wantOK  bool
	}{
		"finds an existing identity by email": {
			seed: func(t *testing.T, r *storage.IdentityRepository) {
				t.Helper()

				if err := r.Create(context.Background(), testutil.KnownIdentity()); err != nil {
					t.Fatalf("seed: %v", err)
				}
			},
			email:  "known@example.com",
			wantOK: true,
		},
		"returns ErrIdentityNotFound when missing": {
			email:   "missing@example.com",
			wantErr: iam.ErrIdentityNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := testutil.NewTestDB(t)
			repo := storage.NewIdentityRepository(postgres.New(pool), testutil.Clock())

			if tc.seed != nil {
				tc.seed(t, repo)
			}

			got, err := repo.FindByEmail(context.Background(), tc.email)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}

			if tc.wantOK {
				if got == nil {
					t.Fatal("expected identity, got nil")
				}

				if got.ID != testutil.KnownIdentityID {
					t.Errorf("id: got %s, want %s", got.ID, testutil.KnownIdentityID)
				}
			}
		})
	}
}
