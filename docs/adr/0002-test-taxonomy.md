# ADR-0002: Test taxonomy and build-tagged integration tests

## Date

2026-05-09

## Context

The API needs unit tests for handler logic, integration tests that exercise the storage layer against real Postgres, and feature tests that drive the wired-up server end-to-end. Where each lives — and how they are gated — affects developer dev-loop time and CI shape.

## Decision

Three test homes:

1. **Unit tests** — `*_test.go` co-located with production code in `api/internal/iam`, no build tag. Run by plain `go test ./...`. Use hand-rolled fakes from `api/internal/testutil`. No Docker, no DB.
2. **Repository integration tests** — `*_integration_test.go` co-located with `api/internal/storage`, gated by `//go:build integration`. Run against a real Postgres testcontainer. Run via `go test -tags=integration ./...`.
3. **Feature tests** — `api/tests/<topic>_test.go`, gated by `//go:build integration`. Wire up the real handlers + real storage + real Postgres; assert HTTP responses and DB side effects.

Test packages always use the `_test` suffix (e.g. `iam_test`, `storage_test`, `tests`) so tests can only access the package's public API.

## Rationale

- **Unit tests stay fast** — no Docker, instantly runnable.
- **Build tag on integration tests** — `go test ./...` doesn't pull in Docker dependencies for routine inner-loop work, but CI runs `-tags=integration` for full coverage.
- **Feature tests live outside `api/internal/`** — the package boundary mechanically enforces public-API-only testing. Placing them in `api/tests/` reflects their cross-cutting nature ("the system as a whole works") rather than tying them to one package.
- **Three layers, not two** — repository tests target the storage layer directly, while feature tests target the wired server. Conflating them would either over-test storage details from the HTTP layer or under-test the wiring at the storage layer.

## Consequences

- New packages adopt the same pattern: unit tests co-located, integration tests behind `//go:build integration`, feature tests in `api/tests/`.
- CI must run `make test-integration` to cover the build-tagged tests.
- Developers iterating on a single package use `go test ./<pkg>/...` for fast feedback; full integration runs are explicit.
