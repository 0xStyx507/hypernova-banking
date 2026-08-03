-- Expired MCP PINs must not remain as dormant authentication factors.
-- Runtime reads also perform this cleanup, while this migration removes any
-- already-expired values when an existing environment is upgraded.
UPDATE users
SET mcp_pin_hash = NULL,
    mcp_pin_expires_at = NULL,
    mcp_pin_failed_attempts = 0,
    mcp_pin_locked_until = NULL,
    updated_at = NOW()
WHERE mcp_pin_expires_at IS NOT NULL
  AND mcp_pin_expires_at <= NOW();
