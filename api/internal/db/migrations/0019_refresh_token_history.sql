-- Keep only hashes of refresh tokens so rotation can detect reuse of a token
-- that was already consumed. Reuse is treated as session compromise.
CREATE TABLE session_refresh_tokens (
    token_hash BYTEA PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    used_at TIMESTAMPTZ
);

CREATE INDEX session_refresh_tokens_user_idx ON session_refresh_tokens (user_id, issued_at DESC);
CREATE INDEX session_refresh_tokens_session_idx ON session_refresh_tokens (session_id, issued_at DESC);
