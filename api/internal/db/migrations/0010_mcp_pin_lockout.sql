-- Limit online guessing of the four-digit MCP confirmation PIN.
ALTER TABLE users ADD COLUMN mcp_pin_failed_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN mcp_pin_locked_until TIMESTAMPTZ;
