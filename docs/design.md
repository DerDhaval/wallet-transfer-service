# Wallet Transfer Service — Technical Design

## 1. Problem statement and assumptions

The service moves money from one wallet to another exactly once, even when clients retry, the process crashes, or many transfers hit the same wallet at the same time.

Assumptions:

- Callers are trusted by the network. Authentication and per-user authorization are out of scope and must sit in front of this service in production.
- Amounts are integers in minor units (paise, cents). `100` means 100 minor units, not 100.00 in major units. Floating-point money is rejected.
- A wallet has one currency. Cross-currency conversion is not supported.
- Wallet creation is a supporting operation so the transfer API can be exercised. Production funding would come from a payment processor; here, `opening_balance` is the funded amount at creation.
- The transfer API is synchronous. The caller receives the terminal state (`PROCESSED` or `FAILED`) from the same request that starts the transfer.
- One idempotency key belongs to one logical request. The key is supplied in the JSON body, as required by the assignment.

## 2. Database schema and relationships

```text
wallets 1 ──< transfers (from_wallet_id)
wallets 1 ──< transfers (to_wallet_id)
transfers 1 ──< ledger_entries (exactly two when PROCESSED, zero when FAILED)
transfers 1 ──< idempotency_records (one request that created the transfer)
wallets 1 ──< ledger_entries
```

| Table | Role |
|---|---|
| `wallets` | Identity, currency, status, `opening_balance`, and the current `balance`. |
| `transfers` | One row per attempt that passed validation and was committed. Status is the state machine. |
| `ledger_entries` | Immutable debit and credit postings. |
| `idempotency_records` | One row per idempotency key, with the request hash and the exact response that was committed. |
| `schema_migrations` | Versions applied by the embedded migration runner (`00000N_create_<table>`). |

Integrity is enforced in the database, not only in Go:

- `wallets.balance >= 0` and `opening_balance >= 0`.
- `transfers.amount > 0` and `from_wallet_id <> to_wallet_id`.
- `UNIQUE (idempotency_key)`.
- `UNIQUE (transfer_id, entry_type)` so a transfer cannot have two debits or two credits.
- Ledger rows cannot be updated or deleted (trigger).
- Transfer financial columns cannot change. Status can move only `PENDING -> PROCESSED` or `PENDING -> FAILED` (trigger).
- A transfer can be inserted only as `PENDING`. A deferred constraint trigger re-reads the row at commit and rejects a transaction that would commit `PENDING`.
- The same deferred trigger requires a `PROCESSED` transfer to have exactly one debit and one credit, each equal to the transfer amount and posted to the source and destination wallets. A `FAILED` transfer must have zero ledger rows.
- A deferred constraint trigger checks `balance = opening_balance + credits - debits` for every wallet whose balance was inserted or updated.
- Ledger insert trigger rejects a posting whose wallet, direction, or amount disagrees with the transfer.

`PENDING` exists so the state machine is real, but it is never visible outside the database transaction. Commit persists only `PROCESSED` or `FAILED`.

## 3. API contract

Amounts are JSON integers. A decimal such as `10.5` is a `400`.

| Method | Path | Success | Purpose |
|---|---|---|---|
| `POST` | `/transfers` | `201`, or `422` when the attempt is recorded as `FAILED` | Create a transfer idempotently. |
| `GET` | `/transfers/{id}` | `200` | Read a transfer, including a recorded failure. |
| `POST` | `/wallets` | `201` | Create a wallet and its opening balance. |
| `GET` | `/wallets/{id}/balance` | `200` | Read the current balance. |
| `GET` | `/wallets/{id}/transactions` | `200` | Ledger history for one wallet (`limit`, `offset`). |
| `GET` | `/health` | `200` | Database ping. |

`POST /transfers` request:

```json
{
  "idempotencyKey": "abc123",
  "fromWalletId": "wallet_1",
  "toWalletId": "wallet_2",
  "amount": 100
}
```

Processed response (`201`):

```json
{
  "transferId": "trf_…",
  "fromWalletId": "wallet_1",
  "toWalletId": "wallet_2",
  "amount": 100,
  "currency": "INR",
  "status": "PROCESSED",
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z"
}
```

`currency` and timestamps are included so clients can reconcile without another lookup. `failureReason` is present only when the status is `FAILED`.

Error body:

```json
{
  "error": {
    "code": "INSUFFICIENT_FUNDS",
    "message": "Not enough balance in the sending wallet to complete this payment"
  },
  "transfer": {
    "transferId": "trf_…",
    "status": "FAILED",
    "failureReason": "INSUFFICIENT_FUNDS"
  }
}
```

`transfer` is present only when a failed transfer row was committed.

| Situation | HTTP | Code | Idempotency key stored? |
|---|---|---|---|
| Malformed JSON, unknown field, missing field | 400 | `INVALID_REQUEST` | No |
| Amount `< 1` or above the maximum | 400 | `INVALID_AMOUNT` | No |
| Same source and destination | 400 | `SAME_WALLET` | No |
| Wallet does not exist | 404 | `WALLET_NOT_FOUND` | No |
| Same key, different payload | 409 | `IDEMPOTENCY_CONFLICT` | No (original record stays) |
| Wallet id already exists | 409 | `WALLET_EXISTS` | n/a |
| Insufficient funds | 422 | `INSUFFICIENT_FUNDS` | Yes, with the failed transfer |
| Wallet not `ACTIVE` | 422 | `WALLET_NOT_ACTIVE` | Yes, with the failed transfer |
| Currency mismatch | 422 | `CURRENCY_MISMATCH` | Yes, with the failed transfer |
| Balance would overflow `int64` | 422 | `AMOUNT_OVERFLOW` | Yes, with the failed transfer |
| Lock timeout, deadlock, serialization failure | 503 | `TEMPORARILY_UNAVAILABLE` | No |
| Any other server or database failure | 500 | `INTERNAL_ERROR` | No |
| Replay of a committed result | Original status | Original body | Already stored |

`422` is used for a well-formed request that cannot be executed because of account state. Syntax problems stay on `400`. A replay adds `X-Idempotent-Replay: true` and returns the stored status and body unchanged.

Client retry rule: on timeout, `500`, or `503`, send the same key and the same body. On `400` or `404`, the key was not consumed and may be reused after the request or the wallet is fixed. On `409`, send the original body to read the original result, or choose a new key for a new transfer. On `201` or `422`, the outcome is final for that key.

## 4. Transfer execution workflow

1. Validate the payload in the process. Invalid input returns before a database transaction starts, so the key is not consumed.
2. Begin a `READ COMMITTED` transaction and set a local lock timeout and statement timeout.
3. Insert the idempotency row with `ON CONFLICT (idempotency_key) DO NOTHING`.
   - If the insert wins, this transaction owns the key.
   - If it loses, compare `request_hash`. Equal hash: return the stored response. Different hash: `409`.
4. Lock both wallets with `SELECT … FOR UPDATE` in sorted wallet-id order.
5. If either wallet is missing, return `404` and roll back (the key insert disappears).
6. If a wallet is not active, the currencies differ, the source balance is too small, or the credit would overflow, insert the transfer as `PENDING`, move it to `FAILED`, store the error response on the idempotency row, and commit. Balances and ledger rows are not written.
7. Otherwise insert `PENDING`, debit the source with `balance = balance - amount` only where `balance >= amount`, credit the destination, insert the debit and credit, move the transfer to `PROCESSED`, store the `201` body, and commit.
8. Deferred triggers run at commit. If they reject the transaction, the database rolls everything back, including the idempotency row.

Request hash is SHA-256 hex of `v1|{fromWalletId}|{toWalletId}|{amount}`. The idempotency key is the lookup, not part of the hash.

## 5. Transaction boundaries

One database transaction contains the idempotency claim, wallet locks, balance updates, transfer status change, ledger inserts, and the stored response.

There is no "money moved, ledger not written" commit. There is no committed `PENDING` transfer.

Failure before the transaction (validation) writes nothing.

Failure during the transaction returns an error from the callback. `Rollback` runs. The caller sees `404`, `409`, `500`, or `503` depending on the cause. A later retry with the same key executes again because the claim was rolled back.

The service also has a test hook that runs after the writes and before `COMMIT`. It exists to prove that a crash at that moment leaves no partial financial data.

## 6. Idempotency strategy

Storage: `idempotency_records.idempotency_key` is unique. The row stores `request_hash`, `transfer_id`, `response_status`, and `response_body`. The body is `TEXT`, not `JSONB`, so Postgres cannot reorder keys and a replay returns the original bytes.

Duplicate detection: the unique index. `INSERT … ON CONFLICT DO NOTHING` either claims the key or yields no row, after which the existing row is loaded.

Original response: the handler writes the stored `response_status` and `response_body` bytes. It does not rebuild the JSON on replay, so later copy changes cannot rewrite history.

Concurrent duplicates: PostgreSQL blocks the second insert on the unique index until the first transaction commits or rolls back. After commit, the second insert conflicts and reads the winner. After rollback, the second insert becomes the owner. Two commits of the same key cannot both create a transfer.

In-flight placeholder: the claim is inserted with `response_status = 0` and a null `transfer_id`. PostgreSQL `CHECK` constraints cannot be deferred, so a deferred constraint trigger re-reads the row at commit and rejects a placeholder that was never completed. Commit requires a real status (`201` or `422`) and a transfer id. Other transactions cannot see the placeholder under `READ COMMITTED`.

Completed idempotency rows are immutable (trigger): the key, hash, transfer id, status, and body cannot change after the status leaves `0`.

Failed business attempts (`422`) are stored on purpose. Repeating them returns the same failure and does not move money. If the customer later adds funds, the client must use a new idempotency key. That removes the ambiguity of "did the first insufficient-funds response actually mean a transfer was created?"

Validation failures and missing wallets are not stored. They are not financial outcomes; the same key can be reused once the caller fixes the request or creates the wallet.

Committed outcome versus a lost HTTP response: if the database commits and the client times out before reading the body, the idempotency row and the transfer are already durable. The retry reads that row and does not post again. A rollback is different: the database has no transfer, no ledger rows, and no idempotency row, so the retry is the first real execution. The unique key makes those two cases distinguishable.

## 7. Concurrency control strategy

Isolation level is `READ COMMITTED`, plus pessimistic row locks.

`SELECT … FOR UPDATE` on each wallet, always in lexicographic id order, serializes transfers that touch the same wallet. The order is mandatory for every future lock path: a transfer `A -> B` and a transfer `B -> A` both lock the lower id first, so they cannot deadlock on each other.

The balance check happens after the lock, so it sees credits and debits that already committed. Two debits of 80 and 50 against a balance of 100 cannot both pass the check.

The debit statement is also conditional: `UPDATE … SET balance = balance - $amount WHERE id = $id AND balance >= $amount AND status = 'ACTIVE'`. The `CHECK (balance >= 0)` constraint is a last barrier. The deferred reconciliation trigger is the proof that the balance still equals the ledger.

`SERIALIZABLE` was considered. It would also prevent the lost update, but it reports conflicts as serialization failures that the application must retry, including for writes that row locks already order cleanly. Pessimistic locks make the outcome explicit: one transaction waits, then sees the updated balance, then succeeds or fails for insufficient funds.

Optimistic version columns were considered and not used. They need a retry loop and still require a conditional update. `FOR UPDATE` matches the critical section (two wallet rows) directly.

Lock wait longer than 5 seconds, a deadlock, or a serialization failure aborts the transaction and returns `503`. The key is not consumed. The client retries the same request.

## 8. Transfer state transitions

```text
PENDING -> PROCESSED
PENDING -> FAILED
```

`PROCESSED` and `FAILED` are terminal. The application updates status only with `WHERE status = 'PENDING'`. The database trigger rejects every other change, including `PROCESSED -> FAILED`.

A processed transfer is never executed again. A replay does not update the transfer, the wallets, or the ledger. It only reads the idempotency row.

Duplicate requests do not cause a second transition. The transfer row is not loaded for update on the replay path.

A failed transfer does not change `wallets.balance` and does not insert ledger rows. The commit-time trigger rejects the transaction if it tries.

## 9. Failure scenarios and recovery behavior

| Failure | What the database does | What the client should do |
|---|---|---|
| Bad JSON, bad amount, same wallet | No transaction | Fix the request. The same key may be reused. |
| Wallet missing | Rollback, including the idempotency claim | Create the wallet and retry the same key. |
| Insufficient funds, inactive wallet, currency mismatch, overflow | Commit a `FAILED` transfer and the idempotency response. No balance change. No ledger rows. | Treat it as final for this key. Use a new key to try a different attempt. |
| Statement error, process crash, or the pre-commit hook | Rollback. No transfer, ledger, balance change, or idempotency row. | Retry the same key and body. |
| Commit succeeded, response lost | Transfer, ledger, balances, and idempotency row are durable. | Retry the same key and body. The stored response is returned and money is not moved again. |
| Lock timeout or deadlock | Rollback | Retry the same key and body. |
| Same key, different body after a committed result | No write | `409`. The first result remains. |

Restarting the process does not replay in-memory work. Anything that was committed is still in Postgres. Anything that was not committed is gone. There is no recovery worker, because this design does not leave `PENDING` transfers behind.

## 10. Consistency guarantees

After every committed transaction:

- A wallet balance is never negative.
- `balance = opening_balance + sum(credits) - sum(debits)` for every wallet whose balance this transaction wrote. Because transfers only move money between wallets, the sum of all balances stays equal to the sum of all opening balances.
- A `PROCESSED` transfer has exactly two immutable ledger entries: a debit on the source and a credit on the destination, for the same amount.
- A `FAILED` transfer has no ledger entries and did not change balances.
- An idempotency key points at most one transfer.
- Replaying that key does not create a second transfer or a second pair of ledger entries.

The stored balance is the value the API reads and the value the debit predicate checks. It is not derived on the read path, because a balance check must be a single locked row, not a scan of the ledger under concurrent writes. The ledger is still required, and the deferred trigger refuses to commit if the two disagree. Deriving balances only from the ledger would also be correct, but it makes the hot path a larger range of rows and makes the non-negative guarantee harder to express as a simple row constraint. The opening balance is stored separately so wallet creation does not need a synthetic system account to satisfy the equation.

## 11. Logging and observability

Logs are text via `log/slog` at info level:

- startup / shutdown
- one access line per HTTP request (`method`, `path`, `status`, `duration_ms`, `request_id`)
- wallet created, transfer processed / failed / replayed
- client rejections as warn; 5xx as error

Request-id and access-log middleware live in `internal/middleware`. Routes are registered in `internal/router`. The database pool and migrations live in `pkg/database`. `GET /health` pings Postgres. The process traps `SIGINT` and `SIGTERM`, stops accepting connections, and closes the pool.

There are no payment credentials in this service. Do not add them to logs later.

## 12. Testing strategy

Unit tests cover validation, the request hash, and lock ordering. They do not need a database.

Integration tests start a dedicated Postgres database, apply the real migrations, and truncate between cases. They cover:

- A successful transfer, both balances, the transfer row, and both ledger entries.
- The ledger/balance equation.
- Repeating one request, then repeating it again after a conflicting payload.
- Concurrent identical requests with one key.
- Concurrent debits that would overdraw the wallet.
- Opposite transfers (`A -> B` and `B -> A`) that would deadlock if locks were taken in request order.
- Missing wallets, bad amounts, same wallet, inactive wallet, currency mismatch.
- A forced error after the writes and before commit, followed by a successful retry of the same key.
- Database triggers: pending cannot commit, processed transfers cannot change status, ledger rows cannot change, and a balance cannot go negative or drift from the ledger.

Handler tests cover JSON decoding and the HTTP status mapping, including one full transfer through `httptest` against Postgres.

Run `go test -race ./...` with the Compose Postgres listening on port `5433`. Packages use separate databases so they can run in parallel without truncating each other.
