-- A confirmation PIN is an ephemeral approval factor. It must be renewed
-- every three minutes and is never returned to the client.
ALTER TABLE users ADD COLUMN mcp_pin_expires_at TIMESTAMPTZ;
