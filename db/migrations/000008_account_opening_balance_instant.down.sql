BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM finance_accounts WHERE opening_balance_at IS NOT NULL)
       OR EXISTS (SELECT 1 FROM finance_opening_confirmations) THEN
        RAISE EXCEPTION 'Refusing to remove confirmed opening balances or their evidence; use a reviewed forward correction or restore';
    END IF;
END $$;
DROP TABLE finance_opening_confirmations;
ALTER TABLE finance_accounts
    DROP CONSTRAINT finance_accounts_opening_instant_shape,
    DROP COLUMN opening_balance_at,
    DROP COLUMN opening_balance_confirmed_at,
    DROP COLUMN balance_revision;
COMMIT;
