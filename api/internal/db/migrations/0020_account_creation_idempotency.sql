-- Account provisioning can be retried after an ambiguous network response.
-- Keep the idempotency intent alongside the account reservation.
ALTER TABLE ledger_accounts
    ADD COLUMN creation_idempotency_key VARCHAR(128),
    ADD COLUMN creation_request_hash BYTEA;

CREATE UNIQUE INDEX ledger_accounts_user_creation_key_idx
    ON ledger_accounts (user_id, creation_idempotency_key)
    WHERE creation_idempotency_key IS NOT NULL;
