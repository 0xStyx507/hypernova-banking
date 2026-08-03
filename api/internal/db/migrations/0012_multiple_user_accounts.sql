-- Users may hold multiple USD checking accounts. Ownership remains enforced
-- by the user_id predicate on every account and financial endpoint.
ALTER TABLE ledger_accounts DROP CONSTRAINT IF EXISTS ledger_accounts_user_currency_unique;
