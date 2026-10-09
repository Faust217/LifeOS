BEGIN;

-- Historical money data must never be silently removed by a rollback.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM finance_accounts)
       OR EXISTS (SELECT 1 FROM finance_categories)
       OR EXISTS (SELECT 1 FROM financial_events) THEN
        RAISE EXCEPTION 'finance data exists; restore a verified backup or use a corrective forward migration';
    END IF;
END $$;

DROP TABLE category_allocations;
DROP TABLE account_movements;
DROP TABLE financial_events;
DROP TABLE finance_categories;
DROP TABLE finance_accounts;

COMMIT;
