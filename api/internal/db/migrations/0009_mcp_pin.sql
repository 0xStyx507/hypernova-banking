-- Store only the bcrypt representation of the user-controlled MCP
-- confirmation PIN. The plaintext PIN is never persisted.
ALTER TABLE users ADD COLUMN mcp_pin_hash TEXT;
