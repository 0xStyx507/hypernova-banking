# Financial ledger

TigerBeetle is the financial source of truth. The API never calculates a
balance from PostgreSQL: it reads the account from TigerBeetle and derives the
posted balance from credits minus debits.

PostgreSQL stores only the information required to operate the API safely:

- account ownership and provisioning status;
- the idempotency key, request hash and TigerBeetle transfer identifier;
- an index of completed transfers for classification and audit queries.

## Monetary representation

The public API accepts and returns positive integer minor units as decimal
strings. The current supported currency is HNL. Floating-point values and
ambiguous decimal amounts are rejected.

## Account invariants

Customer accounts are created in ledger `1` with history enabled and
`DebitsMustNotExceedCredits`. TigerBeetle therefore rejects withdrawals and
transfers that exceed the available posted credits. The API does not perform a
pre-flight balance check because that would introduce a race.

Deposits and withdrawals use a controlled balancing account that is never
exposed through the customer account endpoints. The deposit endpoint is an
explicit local-demo channel controlled by `LEDGER_ALLOW_DEMO_DEPOSITS`; it is
disabled by default in the Go service and must be disabled in production. A
real deployment should replace it with an operator-authorized or provider
webhook workflow.

## Idempotent movements

Deposits, withdrawals and transfers require `Idempotency-Key`. The key is
scoped to the authenticated user and stored with a hash of the complete
financial intent. Reusing a key with a different amount, account or operation
returns `409 Conflict`. Repeating the same request reuses the same immutable
TigerBeetle transfer identifier, including after an uncertain network result.

The database transaction is never held open while calling TigerBeetle. A
retry can reconcile a previously reserved operation because TigerBeetle
returns `TransferExists` for the same transfer identifier and payload.
An uncertain client response is recorded as `unknown`, not as a failed
movement; a recent concurrent request receives `409 Conflict` and an older
unknown reservation can be reconciled with the original identifier.

## Operational limitations

The first ledger release supports HNL checking accounts and recent history
with a maximum page size of 100. History accepts a timestamp cursor and can be
exported as a bounded CSV. The API has a local per-IP rate limit; a distributed
deployment should move that control to a shared edge or gateway.
