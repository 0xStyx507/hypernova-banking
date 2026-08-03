-- The product now operates in USD. Existing development accounts and indexed
-- financial metadata are migrated together so the API never mixes currencies.
ALTER TABLE ledger_accounts DROP CONSTRAINT IF EXISTS ledger_accounts_currency_check;
ALTER TABLE ledger_operations DROP CONSTRAINT IF EXISTS ledger_operations_currency_check;
ALTER TABLE ledger_transfers DROP CONSTRAINT IF EXISTS ledger_transfers_currency_check;
UPDATE ledger_accounts SET currency = 'USD' WHERE currency = 'HNL';
UPDATE ledger_operations SET currency = 'USD' WHERE currency = 'HNL';
UPDATE ledger_transfers SET currency = 'USD' WHERE currency = 'HNL';
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_currency_check CHECK (currency = 'USD');
ALTER TABLE ledger_operations ADD CONSTRAINT ledger_operations_currency_check CHECK (currency = 'USD');
ALTER TABLE ledger_transfers ADD CONSTRAINT ledger_transfers_currency_check CHECK (currency = 'USD');
