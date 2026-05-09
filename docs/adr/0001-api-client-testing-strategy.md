# ADR-0001: Use httptest servers instead of interfaces for API client testing

## Date

2026-04-10

## Context

The web application communicates with the IAM API via `api.IAMClient`, an HTTP client. When testing handlers that depend on this client, we need a strategy for controlling API responses.

Two approaches were considered:

1. **Consumer-defined interfaces** — Each package (e.g., `auth`, `dashboard`) defines an interface with the subset of `IAMClient` methods it uses. Tests mock the interface.
2. **httptest servers** — Tests spin up an `httptest.Server` that stubs API responses. The real `*api.IAMClient` is pointed at the test server.

## Decision

Use `httptest.Server` for testing API client interactions. Packages depend on `*api.IAMClient` directly rather than defining local interfaces.

## Rationale

- **More realistic coverage**: The client *is* the HTTP layer. Mocking it away means tests skip request encoding, response decoding, and error mapping — the most likely sources of bugs.
- **Simpler code**: No need for consumer-defined interfaces, which add an abstraction layer that exists solely for testing.
- **Shared DTO types are low-cost**: The `api` package exposes plain data structs (request/response types) that mirror the external API contract. These are stable and carry no behaviour, so depending on them does not introduce meaningful coupling.
- **No adapter boilerplate**: Fully decoupled interfaces with local types would require adapter structs in `main.go` to map between `api` and consumer types — wiring code with no practical benefit for an internal codebase.

## Consequences

- Packages in `web/internal/` depend directly on `*api.IAMClient` and its request/response types.
- Tests require an `httptest.Server` to stub API responses, which is slightly more setup than a mock but tests a more realistic code path.
- If the web app later develops a rich domain layer, this decision should be revisited — domain packages should define their own interfaces and types, with the API client implementing them.
