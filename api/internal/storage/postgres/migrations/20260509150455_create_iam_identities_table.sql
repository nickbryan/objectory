-- +goose Up
CREATE TABLE "iam"."identities" (
    "id" uuid NOT NULL,
    "email" character varying(255) NOT NULL,
    "password" text NOT NULL,
    "name" character varying(255) NOT NULL,
    "created_at" timestamp NOT NULL,
    "updated_at" timestamp NOT NULL,
    PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "email_unique" ON "iam"."identities" ("email");

-- +goose Down
DROP TABLE "iam"."identities";
