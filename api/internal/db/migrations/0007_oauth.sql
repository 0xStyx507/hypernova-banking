-- OAuth identities are linked by the provider subject, never by email alone.
-- State and exchange artifacts are stored only as hashes so a database read
-- cannot be used to replay an authorization response or obtain a session.
CREATE TABLE oauth_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('google', 'github')),
    provider_subject TEXT NOT NULL,
    provider_email TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_subject),
    UNIQUE (user_id, provider)
);

CREATE INDEX oauth_identities_user_id_idx ON oauth_identities (user_id);

CREATE TABLE oauth_states (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('google', 'github')),
    state_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX oauth_states_expiry_idx ON oauth_states (expires_at);

CREATE TABLE oauth_exchange_codes (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('google', 'github')),
    code_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX oauth_exchange_codes_expiry_idx ON oauth_exchange_codes (expires_at);
