-- Bind the second factor to the session that completed it.
ALTER TABLE sessions ADD COLUMN mfa_verified_at TIMESTAMPTZ;

CREATE INDEX sessions_mfa_verified_idx
    ON sessions (user_id, mfa_verified_at)
    WHERE revoked_at IS NULL AND mfa_verified_at IS NOT NULL;
