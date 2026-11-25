# AI usage

## Tool

**Cursor IDE — Composer 2.5**

The service (code, migrations, tests, docs, and Postman) was implemented in Cursor with Composer 2.5. Design choices and final review remain the author’s responsibility.

## How it was used

- Assignment requirements → design note (`docs/design.md`) → implementation → tests.
- Composer generated layered Go/PostgreSQL code, SQL migrations, table-driven tests, and API docs.
- The author reviewed diffs, fixed env/runtime issues, and ran `go test` / `go test -race` against real Postgres.

## Example prompts

1. Implement idempotent `POST /transfers` with a double-entry ledger, concurrency safety, layered architecture, tests, and design docs.
2. Match project layout (`pkg/database`, router/middleware, one-table up/down migrations).
3. Prefer table-driven tests and shared `tests/testutil` helpers.
4. Add a single Postman collection and `docs/api-test-cases.md`.
5. Improve API error messages; simplify code without dropping features.
6. Audit the repo against the assignment checklist.

## What stayed with the author

- Running and interpreting tests on real Postgres.
- Accepting or rejecting design tradeoffs (stored balance, ledger, ordered `FOR UPDATE` — see `docs/design.md`).
- Explaining the PR and answering review questions.
