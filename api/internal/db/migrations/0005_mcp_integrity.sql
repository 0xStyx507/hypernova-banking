-- A deterministic intent hash detects payload tampering between preparation
-- and confirmation. It remains nullable only for actions created before this
-- migration; the service backfills those legacy rows after validation.
ALTER TABLE mcp_prepared_actions ADD COLUMN payload_hash BYTEA;
