# AI Usage & Collaboration Note

This document summarizes the use of AI tools during the design, implementation, and verification of the Wallet Transfer Service, in accordance with the assignment guidelines.

---

## 1. Tool Used

* **Cursor /composer 2.5** (IDE-integrated coding assistant)

---

## 2. Methodology & Workflow

AI was utilized as an **accelerator and pair-programming assistant** rather than an autonomous decision-maker:

1. **Architecture & Design**: Core architectural patterns, transactional boundaries, and database schemas were planned and outlined prior to code generation.
2. **Scaffolding & Boilerplate**: AI was leveraged to generate repetitive structures (HTTP handler boilerplate, DTO mappings, SQL migration templates, and comprehensive table-driven test cases).
3. **Edge-Case Brainstorming**: AI was used to probe for financial edge cases (such as delimiter collision in request hashing, race conditions during simultaneous transfers, and floating-point pitfalls).
4. **Iterative Verification**: Every generated component was reviewed, manually audited, and validated against PostgreSQL integration tests and Postman test suites.

---

## 3. Key Prompt Categories & Intent

Work was conducted through focused, modular prompts across development phases:

1. **Schema & Migration Design**:
   - Drafted PostgreSQL DDL for `wallets`, `transfers`, `ledger_entries`, and `idempotency_records` with integrity constraints and trigger templates.
2. **Domain & Business Logic**:
   - Implemented domain entities, state transitions (`PENDING -> PROCESSED | FAILED`), and deadlock-free lock ordering (`OrderedWalletIDs`).
   - Enforced single-currency transfers with exact zero-sum double-entry ledger parity (`1 Debit = 1 Credit`).
3. **Idempotency & Concurrency Strategy**:
   - Designed canonical length-prefixed request hashing (`RequestHash`) to prevent delimiter collision.
   - Implemented atomic claim/complete workflows using PostgreSQL `ON CONFLICT` constraints.
4. **Testing & Quality Assurance**:
   - Scaffolding unit and integration tests (table-driven tests, boundary checks, and concurrent transfer simulations).
   - Generated structured Postman collections covering happy paths, idempotency replays, validation failures, and edge cases.
5. **Code Polish & Refactoring**:
   - Refactored HTTP response utilities (`writeJSON`, `writeError`) and ensured strict separation across Clean Architecture layers.

---

## 4. Engineering Ownership & Verification

All core design decisions, system invariants, and code paths were verified by the developer:
* Selection of `READ COMMITTED` isolation combined with ordered `SELECT ... FOR UPDATE` row locks.
* Invariant enforcement in PostgreSQL deferred triggers (`assert_wallet_balance`).
* Validation of server graceful shutdown, timeouts, and resource cleanup.
* Full execution and verification of all unit, integration, and Postman test suites.
