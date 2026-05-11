# ADR-0004: Testing conventions for the API module

## Date

2026-05-09

## Context

We are establishing the project's first test suite. The conventions chosen here will set expectations for every subsequent test author. The user has been explicit about what they want — and what they don't want — based on past experience with mocking-framework rot, fragile assertions, and slow suites.

## Decision

The following conventions apply to all tests under `api/`:

1. **Hand-rolled fakes only.** No mocking frameworks (`testify/mock`, `gomock`, etc.). Fakes live in `api/internal/testutil` and follow a hybrid pattern: an in-memory implementation that mirrors real behaviour by default, with public fields for per-test error injection.
2. **Comparisons use `github.com/google/go-cmp/cmp` only.** No `reflect.DeepEqual`, no `testify/assert`.
3. **Response bodies asserted as JSON literals.** The `testutil.JSONResponse` and `testutil.ProblemResponse` helpers take a `wantJSON string`, decode both the actual and expected to `any`, and `cmp.Diff` the result. Tests read like the wire contract.
4. **Table-driven cases are `map[string]struct{...}`.** Nested `t.Run(name, …)`. When tabular form hurts readability (multi-step flows, sequenced setup), nested `t.Run` calls without a table are fine.
5. **Public-API testing only.** Test packages use the `_test` suffix (`iam_test`, `storage_test`, `tests`) so they can only see exported identifiers.
6. **Determinism via injection, not fuzziness.** When code reads time, UUIDs, or random values, the test injects a deterministic source via the package's seam (clock function, UUID generator, etc.). No `time.Now() +/- 5s` tolerance assertions.
7. **`t.Parallel()` everywhere.** Tests must not depend on order; `-shuffle=on` is the default Make target.
8. **In-memory logger, never discarded.** Tests use `slogutil.NewInMemoryLogger(level)` so log records remain inspectable; never wire `slog.NewJSONHandler(io.Discard, …)`. Where applicable, assert on log records via `slogmem.RecordQuery`.

## Rationale

Each rule prevents a specific failure mode:

- **No mocking frameworks**: removes the "fake matched but real diverged" failure class. Hand-rolled fakes that wrap the production interface stay synchronised by the compiler.
- **`cmp.Diff` everywhere**: a single, readable diff format. `testify/assert` produces inconsistent failure output across packages.
- **JSON-literal assertions**: matches the wire format the API documents publish. Reviewers don't need to mentally translate Go map literals.
- **`map[string]struct{...}`**: random iteration order surfaces ordering bugs immediately. Slice-of-cases would let an ordering bug slip through.
- **Public API only**: tests stay aligned with consumer reality; refactors of unexported helpers don't break the suite.
- **Determinism**: brittleness from `time.Now()` and random UUIDs is the single largest source of flaky tests in a JWT/identity codebase.
- **In-memory logger**: lets a debugging engineer inspect what the system logged for a failing case without re-running with a different harness.

## Consequences

- All future packages follow these conventions. Linters (`golangci-lint`) should enforce no `testify/mock` imports.
- New helpers added to `testutil` should follow the hybrid in-memory + error-injection shape unless there's a specific reason not to.
- If a test ever needs fuzzy time matching (e.g. asserting "this happened at most N seconds ago"), prefer fixing the production seam first. Only fall back to fuzziness when injection genuinely cannot reach the call site.
- Feature tests in `api/tests/` are an exception to the deterministic-clock rule for the `iam.Endpoints` `now` parameter: the JWT guard validates `iat`/`exp` against `time.Now()` internally, so a fixed clock would issue tokens outside the guard's acceptance window. These tests use the real wall clock and do not assert on exact timestamps.
