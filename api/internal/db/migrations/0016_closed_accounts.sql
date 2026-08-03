-- Closing is a reversible domain state, not a destructive delete. The
-- TigerBeetle account and its audit references remain available for review.
ALTER TABLE ledger_accounts DROP CONSTRAINT IF EXISTS ledger_accounts_status_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_status_check
  CHECK (status IN ('provisioning', 'active', 'failed', 'closed'));
