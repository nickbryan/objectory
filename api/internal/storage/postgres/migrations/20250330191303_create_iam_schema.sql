-- Drop schema named "public"
DROP SCHEMA "public" CASCADE;
-- Add new schema named "iam"
CREATE SCHEMA "iam";
-- Set comment to schema: "iam"
COMMENT ON SCHEMA "iam" IS 'The identity and access management database.';
