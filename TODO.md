# TODO

## CI

- [ ] Set up `.github/workflows/` (lint, test) for the project.
- [ ] Wire **squawk** into CI to lint migration safety on PRs that touch `api/internal/storage/postgres/migrations/`. Use `sbdchd/squawk-action` or install the binary directly. Optionally add a `make squawk` target alongside for local runs.
