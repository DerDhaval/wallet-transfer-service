# API test cases (requirement + corner cases)

Use with the Postman collection in `postman/`.

**Prereq:** API running (`go run ./cmd/server`).

Import the single file `postman/Wallet_Transfer_Service.postman_collection.json`, then run folder **00 — Setup** first (or Run collection). Variables live on the collection — no separate environment file.

Amounts are integer **minor units**.

---

## 1. Requirement cases

| ID | Case | Request | Expect |
|---|---|---|---|
| R1 | Health | `GET /health` | `200` `{status:ok}` |
| R2 | Create funded wallet | `POST /wallets` opening 50000 | `201` balance 50000 |
| R3 | Create empty wallet | `POST /wallets` opening 0 | `201` balance 0 |
| R4 | Successful transfer | `POST /transfers` amount 100 | `201` `PROCESSED`, debit/credit |
| R5 | Source balance after transfer | `GET /wallets/{A}/balance` | `200` balance 49900 |
| R6 | Dest balance after transfer | `GET /wallets/{B}/balance` | `200` balance 100 |
| R7 | Get transfer | `GET /transfers/{id}` | `200` same amount/status |
| R8 | Ledger history | `GET /wallets/{A}/transactions` | `200` DEBIT 100 |
| R9 | Idempotent replay | same key + same body | `201` + `X-Idempotent-Replay: true`, balances unchanged |
| R10 | Exactly-once money move | after replay, balances still 49900 / 100 | no second debit |

---

## 2. Idempotency cases

| ID | Case | Expect |
|---|---|---|
| I1 | Same key, same payload | original result, replay header |
| I2 | Same key, different amount | `409 IDEMPOTENCY_CONFLICT` |
| I3 | New key after conflict | can transfer again with new key |
| I4 | Insufficient funds then replay | `422` stored; replay same `422` body, no balance change |

---

## 3. Validation / corner cases

| ID | Case | Expect |
|---|---|---|
| C1 | Same source and destination | `400 SAME_WALLET` |
| C2 | Amount `0` | `400 INVALID_AMOUNT` |
| C3 | Amount negative | `400 INVALID_AMOUNT` |
| C4 | Fractional amount `10.5` | `400 INVALID_REQUEST` |
| C5 | Missing wallet | `404 WALLET_NOT_FOUND` |
| C6 | Empty body | `400 INVALID_REQUEST` |
| C7 | Unknown JSON field | `400 INVALID_REQUEST` |
| C8 | Wrong Content-Type | `415 UNSUPPORTED_MEDIA_TYPE` |
| C9 | Duplicate wallet id | `409 WALLET_EXISTS` |
| C10 | Missing transfer id | `404 TRANSFER_NOT_FOUND` |
| C11 | Bad transactions `limit` | `400 INVALID_REQUEST` |

---

## 4. Business failure cases

| ID | Case | Expect |
|---|---|---|
| B1 | Transfer more than balance | `422 INSUFFICIENT_FUNDS` + `transfer.status=FAILED` |
| B2 | Failed transfer does not move money | balances unchanged |
| B3 | Failed transfer has no ledger impact | history unchanged for that attempt |

| B4 | Amount too small after conversion | `422 AMOUNT_TOO_SMALL`, no balance change |
| B5 | Wallet currency not supported | `422 CURRENCY_NOT_SUPPORTED`, no balance change |

---

## 5. Currencies and cross-currency transfers

| ID | Case | Expect |
|---|---|---|
| X1 | `GET /currencies` | `200`, 100+ currencies including INR, USD, JPY, KWD |
| X2 | `GET /convert?from=INR&to=USD&amount=10000` | `200`, `toAmount` 112 |
| X3 | Convert with unsupported currency | `400 INVALID_REQUEST` |
| X4 | Create wallet with unsupported currency | `400 INVALID_REQUEST` |
| X5 | Create USD wallet | `201` |
| X6 | Transfer 10000 paise INR to the USD wallet | `201`, `toAmount` 112, `toCurrency` USD, `exchangeRate` stored |
| X7 | USD wallet balance | `200`, balance 112 |
| X8 | Replay X6 | `201` + replay header, same conversion, no second move |
| X9 | Transfer 1 paisa INR to USD | `422 AMOUNT_TOO_SMALL` |

---

## 6. Collection run order

1. **00 — Setup** (sets `runId`, wallet ids, keys)
2. **01 — Requirement happy path**
3. **02 — Idempotency**
4. **03 — Validation corner cases**
5. **04 — Business failures**
6. **05 — Lookup corner cases**
7. **06 — Currencies and cross-currency transfers**

Each request has Postman `pm.test` assertions. Failures show in the Test Results tab.
