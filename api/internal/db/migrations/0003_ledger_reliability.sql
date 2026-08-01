-- Migration 0002 was applied by some local databases before unknown ledger
-- outcomes were introduced. Keep those databases compatible with retries.
ALTER TABLE ledger_operations DROP CONSTRAINT ledger_operations_status_check;
ALTER TABLE ledger_operations
    ADD CONSTRAINT ledger_operations_status_check
    CHECK (status IN ('processing', 'unknown', 'succeeded', 'failed'));
