-- TOTP enrollment is encrypted at rest and remains disabled until the user
-- proves possession of a compatible authenticator application.
ALTER TABLE users
    ADD COLUMN mfa_secret_encrypted BYTEA,
    ADD COLUMN mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN mfa_enrollment_expires_at TIMESTAMPTZ;
