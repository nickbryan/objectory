# ADR-0005: Configurable seams in `iam.Endpoints`

## Date

2026-05-09

## Context

The `iam` package's handlers depend on three sources of nondeterminism: the current time (used for JWT `iat`/`exp` and DB timestamps), random UUIDs (identity IDs and JWT `jti`), and bcrypt cost (which is wall-clock-bound at default cost). Tests need each of these to be deterministic; production wants real values.

Two design directions were available: (a) inject each as a parameter at the package's public constructor, or (b) keep production code reaching for global `time.Now`/`uuid.NewRandom` and use `testing/synctest` plus precomputed test fixtures to control behaviour.

## Decision

Inject all three at `iam.Endpoints`:

```go
func Endpoints(
    logger *slog.Logger,
    uuidV4Generator UUIDV4Generator,
    identityRepository IdentityRepository,
    jwtKey string,
    passwordCost int,
    now func() time.Time,
) httputil.EndpointGroup
```

The same `UUIDV4Generator` and `now` are plumbed into both `identityCreateHandler` and `tokenCreateHandler`.

The `storage.IdentityRepository` constructor already takes a `now func()`; the same convention now applies to `iam.Endpoints`.

## Rationale

- **One mechanism across the codebase.** Both `iam` and `storage` constructors take `now func() time.Time`; new packages will follow. A consistent pattern is easier to learn than a mix of injection here / `synctest` bubble there.
- **No durably-blocked-goroutine caveats.** `testing/synctest` doesn't treat real network I/O as durably blocked, so it can't be used in DB-touching tests. Injection works at every layer.
- **Deterministic JWT assertions.** Unit tests compare entire JWT claim structs with `cmp.Diff` against an exact expected value, including `iat`, `exp`, and `jti` — caught by clock and UUID injection respectively.
- **Bcrypt cost as a knob.** `bcrypt.MinCost` (4) takes ~1ms per hash; `bcrypt.DefaultCost` (10) takes ~75ms. For ~30 hashes across the integration suite, that's ~2s saved per CI run.

## Consequences

- `iam.Endpoints` has six parameters. At the edge of comfortable; if a future addition pushes to seven, collapse into a `Config` struct.
- `main.go` is the single place that passes the production values (`time.Now`, `bcrypt.DefaultCost`, `uuidgen.New()`). Code review on changes to those lines provides the safety net for regressions.
- Unit tests pass `bcrypt.MinCost` directly; fixtures pre-compute hashes at the same cost so password-matching round-trips work without paying the default-cost penalty.
- The injected clock is not consulted by `NewJWTGuard` — jwt-go validates `iat`/`exp` against `time.Now()` internally. Feature tests that issue a token via `tokenCreateHandler` and then call a guarded endpoint must use the real wall clock for `now` (see ADR-0004 §Consequences).
- `synctest` remains available as a tool for future tests that need to *advance* fake time mid-test (rate-limit windows, refresh-token expiry tests). It's not the default approach for "what value gets stamped here."
