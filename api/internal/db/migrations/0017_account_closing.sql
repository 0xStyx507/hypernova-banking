-- Closing is a transient state used to serialize account shutdown with
-- operations that may already be reserved in PostgreSQL.
ALTER TABLE ledger_accounts DROP CONSTRAINT IF EXISTS ledger_accounts_status_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_status_check
  CHECK (status IN ('provisioning', 'active', 'closing', 'failed', 'closed'));
