-- Bound repeated TOTP guesses even when an attacker rotates source IPs within
-- a single API instance. The lock is reset after a successful verification.
ALTER TABLE users
    ADD COLUMN mfa_failed_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN mfa_locked_until TIMESTAMPTZ;
