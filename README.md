# Wallet Transfer Service

A small Go API that moves money from one wallet to another. A transfer is committed with its balance updates and its two ledger entries, or it is not committed at all. Repeating a request does not move the money twice.

Amounts are integers in minor units. `100` means 100 paise or cents, not 100.00.

The design, including locking, idempotency, and failure behavior, is in [docs/design.md](docs/design.md). The HTTP contract is also in [docs/openapi.yaml](docs/openapi.yaml).

## What the service guarantees

- A wallet balance never goes negative.
- `balance = opening_balance + credits - debits`.
- A processed transfer has exactly one debit and one credit, for the same amount.
- A failed transfer does not change balances and does not write ledger rows.
- The same idempotency key and the same body always return the original response.
- The same key with a different body returns `409 Conflict`.
- Concurrent debits cannot overdraw a wallet. Locks are taken in wallet-id order so opposite transfers do not deadlock.

## Run it

Requirements: Go 1.23+, Docker, and the `docker-compose` command.

```bash
docker-compose up -d postgres
export DATABASE_URL='postgres://wallet:wallet@127.0.0.1:5433/wallet_transfer?sslmode=disable'
go run ./cmd/server
```

Postgres is published on **host port 5433** so it does not collide with a Postgres already listening on 5432. The API listens on `:8080`. Migrations run on startup (`*.up.sql`, one table per file, with matching `*.down.sql`).

The API and Postgres can also be started together:

```bash
docker-compose up --build
```

### Example

```bash
curl -sS -X POST localhost:8080/wallets \
  -H 'Content-Type: application/json' \
  -d '{"id":"wallet_1","currency":"INR","initialBalance":50000}'

curl -sS -X POST localhost:8080/wallets \
  -H 'Content-Type: application/json' \
  -d '{"id":"wallet_2","currency":"INR","initialBalance":0}'

curl -sS -X POST localhost:8080/transfers \
  -H 'Content-Type: application/json' \
  -d '{"idempotencyKey":"abc123","fromWalletId":"wallet_1","toWalletId":"wallet_2","amount":100}'
```

A processed transfer responds with `201`:

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

Sending that request again returns the same body and status, with `X-Idempotent-Replay: true`. `wallet_1` is debited once.

## API

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/transfers` | Transfer money. Idempotent. |
| `GET` | `/transfers/{id}` | Read a transfer. |
| `POST` | `/wallets` | Create a wallet and its opening balance. |
| `GET` | `/wallets/{id}/balance` | Read a balance. |
| `GET` | `/wallets/{id}/transactions` | Ledger entries, newest first. `limit` (default 20, max 100) and `offset`. |
| `GET` | `/health` | Database ping. |

`POST /transfers` body:

| Field | Rule |
|---|---|
| `idempotencyKey` | Required. 1–255 characters: letters, digits, `.`, `_`, `:`, `-`. |
| `fromWalletId`, `toWalletId` | Required, different, and must already exist. |
| `amount` | Integer minor units, from 1 to 1_000_000_000_000. |

### Status codes

| Situation | Status | Code |
|---|---|---|
| Malformed JSON, unknown field, bad id | 400 | `INVALID_REQUEST` |
| Zero, negative, or too-large amount | 400 | `INVALID_AMOUNT` |
| Same source and destination | 400 | `SAME_WALLET` |
| Wallet or transfer does not exist | 404 | `WALLET_NOT_FOUND` or `TRANSFER_NOT_FOUND` |
| Same key, different body | 409 | `IDEMPOTENCY_CONFLICT` |
| Wallet id already used | 409 | `WALLET_EXISTS` |
| Insufficient funds, inactive wallet, currency mismatch, balance overflow | 422 | `INSUFFICIENT_FUNDS`, `WALLET_NOT_ACTIVE`, `CURRENCY_MISMATCH`, `AMOUNT_OVERFLOW` |
| Lock timeout or deadlock | 503 | `TEMPORARILY_UNAVAILABLE` |
| Unexpected failure | 500 | `INTERNAL_ERROR` |

`422` means the attempt was saved as `FAILED`. The same key returns that failure again. Use a new key to try again after the customer adds funds. `400`, `404`, `500`, and `503` do not consume the key.

Error body:

```json
{"error":{"code":"INSUFFICIENT_FUNDS","message":"source wallet has insufficient funds"}}
```

A committed business failure also includes the `transfer` object.

## Tests

Start Postgres, then run the suite with the race detector:

```bash
docker-compose up -d postgres
make test
make cover
```

`make test` is `go test -race -count=1 ./...`. `make cover` reports statement coverage across `./internal/...` (target: about 80%+). Integration tests create their own databases and drop them afterward. They need the Compose Postgres on port 5433, or `DATABASE_URL` pointed at a Postgres the process can create databases in.

Tests are mostly table-driven and live next to each package (`*_test.go`): domain validation, config, middleware, router, handler HTTP cases, repository SQL paths, and service transfer/wallet behavior including concurrency and idempotency.

## Configuration

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `DATABASE_URL` | yes |  | Postgres URL. |
| `HTTP_ADDR` | no | `:8080` | Listen address. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. |

Logs are JSON. They include transfer ids, wallet ids, amounts, idempotency keys, and request ids. Do not add credentials to them.

## Layout

```text
cmd/server/main.go              process entrypoint
internal/
  router/                       HTTP route registration
  middleware/                   request id + access log
  handler/                      HTTP request/response adapters
  service/                      business logic and idempotency
  repository/                   SQL, row locks, transactions
  domain/                       entities, validation, errors
  config/                       env config
pkg/
  database/                     Postgres pool + migrations
migrations/                     SQL schema files
tests/testutil/                 Postgres helpers for integration tests
docs/                           design + OpenAPI
```

Call path: `main → router → middleware → handler → service → repository → pkg/database`.

## AI usage

See [docs/AI_USAGE.md](docs/AI_USAGE.md).
