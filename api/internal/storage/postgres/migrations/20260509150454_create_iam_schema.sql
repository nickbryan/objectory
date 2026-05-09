-- +goose Up
CREATE SCHEMA "iam";
COMMENT ON SCHEMA "iam" IS 'The identity and access management database.';

-- +goose Down
DROP SCHEMA "iam" CASCADE;
