-- name: CreateIdentity :exec
INSERT INTO iam.identities (id, email, password, name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: Identity :one
SELECT * FROM iam.identities
WHERE id = $1;

-- name: IdentityByEmail :one
SELECT * FROM iam.identities
WHERE email = $1;
