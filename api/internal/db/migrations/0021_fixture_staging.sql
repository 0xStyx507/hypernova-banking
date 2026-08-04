-- Development fixture staging keeps source data queryable without making
-- PostgreSQL a second balance authority. A later replay use case can create
-- TigerBeetle transfers from these validated rows explicitly.
CREATE TABLE IF NOT EXISTS fixture_accounts (
    account_number TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    initial_balance_minor BIGINT NOT NULL CHECK (initial_balance_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency = 'USD'),
    account_type TEXT NOT NULL CHECK (account_type IN ('checking', 'savings')),
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS fixture_transactions (
    source_key TEXT PRIMARY KEY,
    from_account TEXT NOT NULL,
    to_account TEXT NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    operation_type TEXT NOT NULL CHECK (operation_type IN ('deposit', 'withdrawal', 'transfer')),
    description TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('completed', 'pending', 'failed')),
    currency CHAR(3) NOT NULL CHECK (currency = 'USD'),
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS fixture_transactions_occurred_idx
    ON fixture_transactions (occurred_at DESC);
