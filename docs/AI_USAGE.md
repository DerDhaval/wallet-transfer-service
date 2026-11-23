# AI usage

## Tools used

Cursor, with the Grok coding agent, was used while building this service.

## How it was used

The assignment text was the development prompt. The agent was asked to check the requirements and implement the service, including the design note, PostgreSQL schema, transfer API, idempotency, concurrency control, and tests.

The design choices in `docs/design.md` are the ones the implementation follows: stored balances plus an immutable double-entry ledger, one database transaction per transfer, a unique idempotency key, and wallet locks taken in sorted id order. Those choices should be explainable from the code and the tests, not only from the chat.

AI output was compiled and run with `go test -race ./...` against PostgreSQL. Failures from that run were fixed in the code rather than left as generated suggestions.

## Prompts

The working prompt was the assignment in full, followed by the instruction to check it and implement it properly, including the tests. No separate prompt list was used.

## Session transcript

The session is the Cursor chat that produced this repository. The raw agent log also contains tool traces and product instructions that are not part of the solution, so it is not copied into the repo. The prompt above, this note, and the resulting code are the record of how the tool was used.
