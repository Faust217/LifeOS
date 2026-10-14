BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM financial_events WHERE deleted_at IS NOT NULL) THEN
        RAISE EXCEPTION 'deleted financial events exist; use a corrective forward migration or verified backup';
    END IF;
END $$;

ALTER TABLE financial_events DROP COLUMN deleted_at;

COMMIT;
