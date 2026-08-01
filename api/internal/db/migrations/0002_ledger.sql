-- Financial metadata belongs in PostgreSQL, while monetary state remains in
-- TigerBeetle. This split keeps ownership and replay controls queryable
-- without creating a second balance authority.
CREATE TABLE ledger_accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    tigerbeetle_account_id TEXT NOT NULL UNIQUE,
    account_type TEXT NOT NULL DEFAULT 'checking',
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL DEFAULT 'provisioning',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ledger_accounts_type_check CHECK (account_type IN ('checking')),
    CONSTRAINT ledger_accounts_currency_check CHECK (currency = 'HNL'),
    CONSTRAINT ledger_accounts_status_check CHECK (status IN ('provisioning', 'active', 'failed')),
    CONSTRAINT ledger_accounts_user_currency_unique UNIQUE (user_id, currency)
);

CREATE INDEX ledger_accounts_user_idx ON ledger_accounts (user_id, status);

CREATE TABLE ledger_operations (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    idempotency_key VARCHAR(128) NOT NULL,
    request_hash BYTEA NOT NULL,
    operation_type TEXT NOT NULL,
    tigerbeetle_transfer_id TEXT NOT NULL UNIQUE,
    debit_account_id TEXT NOT NULL,
    credit_account_id TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency CHAR(3) NOT NULL CHECK (currency = 'HNL'),
    status TEXT NOT NULL DEFAULT 'processing',
    error_code TEXT,
    response_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ledger_operations_type_check CHECK (operation_type IN ('deposit', 'withdrawal', 'transfer')),
    CONSTRAINT ledger_operations_status_check CHECK (status IN ('processing', 'unknown', 'succeeded', 'failed')),
    CONSTRAINT ledger_operations_user_key_unique UNIQUE (user_id, idempotency_key)
);

CREATE INDEX ledger_operations_user_created_idx ON ledger_operations (user_id, created_at DESC);

CREATE TABLE ledger_transfers (
    tigerbeetle_transfer_id TEXT PRIMARY KEY,
    operation_id UUID NOT NULL UNIQUE REFERENCES ledger_operations(id),
    user_id UUID NOT NULL REFERENCES users(id),
    debit_account_id TEXT NOT NULL,
    credit_account_id TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency CHAR(3) NOT NULL CHECK (currency = 'HNL'),
    operation_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ledger_transfers_user_created_idx ON ledger_transfers (user_id, created_at DESC);
