# TODO

## CI

- [ ] Set up `.github/workflows/` (lint, test) for the project.
- [ ] Wire **squawk** into CI to lint migration safety on PRs that touch `api/internal/storage/postgres/migrations/`. Use `sbdchd/squawk-action` or install the binary directly. Optionally add a `make squawk` target alongside for local runs.

## Auth — security hardening

- [ ] **CSRF protection** on the web form POSTs (`/actions/login`, `/actions/register`, `/actions/logout`). Cookie is `SameSite=Lax` which mitigates cross-site POSTs but is not a full substitute — add a synchronizer-token (or double-submit) check in `web/internal/auth`. Apply to GET-rendered forms in `views/partials/forms/` so the token round-trips through HTMX swaps.
- [ ] **Rate limiting / brute-force protection** on `POST /iam/tokens` (api) and `/actions/login` (web). Per-IP and per-email buckets; consider account lockout / exponential backoff after N failures. Decide where this lives (middleware in `api/internal/iam` vs. an edge layer in Caddy).
- [ ] **JWT revocation on logout.** Today `sessionDeleteHandler` only clears the cookie; the JWT remains valid until its 24h expiry if it was captured. Two options: (a) shorten access-token TTL and add refresh tokens, or (b) add a server-side revocation list keyed off the `jti` already generated in `api/internal/iam/token.go`. Pick one and implement.
- [ ] **Failed-login lockout / audit logging.** Track failed `POST /iam/tokens` attempts per identity and lock the account temporarily after a threshold. Emit structured audit events for login success, login failure, registration, logout, and lockout.

## Auth — user-facing features

- [ ] **Email verification on signup.** Generate a one-time verification token at identity creation, send via email, gate login (or gate sensitive actions) on verified status. Needs an email transport — decide on provider/abstraction first.
- [ ] **Password reset flow.** "Forgot password" form → time-limited reset token emailed → reset form that consumes the token. Reuse whatever email transport is chosen for verification. Needs new endpoints on the API and matching web forms/partials.
- [ ] **"Remember me" / session extension.** Currently the JWT and cookie both hard-expire at 24h with no renewal. Either add a refresh-token endpoint or extend the cookie/JWT lifetime when the user opts in at login.

## Auth — polish

- [ ] **Cookie `Secure` flag in dev.** `setAuthCookie` / `ClearAuthCookie` in `web/internal/auth/login.go` always set `Secure: true`. Fine behind Caddy (which terminates TLS), but confirm there is no local non-TLS path that silently drops the cookie. If there is, derive the flag from config rather than hardcoding.
- [ ] **Login error differentiation in logs.** `sessionCreateHandler` returns a single generic "Invalid email or password." string regardless of cause — keep that for the user, but make sure logs distinguish "identity not found" from "wrong password" from "API error" so debugging is possible.
