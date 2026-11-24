# AI usage

## 1. Tool used

**Cursor** (Composer / Grok coding agent in the Cursor IDE).

## 2. How the tool was generally used

- Paste or reference the assignment and ask for a gap check against the repo.
- Ask for design/docs first (`docs/design.md`), then implementation, then tests.
- Use the agent for boilerplate, migrations, Postman, and refactors; review and run `go test` / `go test -race` locally.
- Fix env/runtime issues (`.env`, Go toolchain, Postgres credentials) with the agent, but keep design decisions explainable from code + `docs/design.md`.
- Prefer small, reviewable changes over one blind “build everything” dump.

## 3. Prompts used in this work (session summary)

Exact chat turns varied; these are the prompts / intents that drove the solution:

1. Implement the full wallet-transfer assignment (Go + PostgreSQL): idempotent `POST /transfers`, double-entry ledger, concurrency, layered architecture, tests, design/AI docs.
2. Align project structure with an Orthoheels-style layout (`pkg/database`, router/middleware separate, one-table up/down migrations).
3. Prefer ~80%+ table-driven tests and shared `tests/testutil` helpers.
4. Add Postman collection and API test-case docs; then merge into a **single** collection file (no separate environment file).
5. Make `.env` load on `go run` (godotenv) and use local Postgres (`dhaval` / `5450` / `wallet` on `:5432`).
6. Quiet terminal logs — drop noisy JSON/access logs and `LOG_LEVEL`.
7. Improve user-facing API `message` text for all error / payment cases.
8. Code cleanup: remove duplication, keep the same features, senior-style simplicity.
9. Audit the finished repo against the assignment text (this check).
10. Support transfers between wallets of different currencies: add all ISO 4217 currencies, a converter with exact rounding (never creates money), store the rate on each transfer, and add `/currencies` and `/convert`.

## 4. Session transcript

The full Cursor agent transcript for this project lives in the local Cursor project data (agent transcripts), not as a public paste in git (it can include tool traces and local paths).

For review / discussion:

- Use this note + `docs/design.md` + the PR description for **what was decided and why**.
- Use `git log` and the code for **what shipped**.
- If reviewers need the raw chat export, it can be emailed or attached separately from Cursor’s agent transcript for this workspace.

## 5. What AI did *not* replace

- Running and interpreting `go test` / race tests against real Postgres.
- Choosing stored balance + ledger + ordered `FOR UPDATE` (justified in `docs/design.md`).
- Explaining the PR in a live discussion — that remains the author’s responsibility.
