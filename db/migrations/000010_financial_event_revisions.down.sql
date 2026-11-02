BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM financial_events) OR EXISTS (SELECT 1 FROM financial_event_revisions)
       OR EXISTS (SELECT 1 FROM finance_write_requests WHERE fingerprint_version > 1) THEN
        RAISE EXCEPTION 'Refusing to discard financial revision or retry evidence; use a forward correction';
    END IF;
END $$;
DROP FUNCTION capture_financial_event_revision(uuid,uuid,text,uuid);
DROP TABLE financial_event_revisions;
DROP FUNCTION reject_financial_revision_mutation();
ALTER TABLE financial_events DROP COLUMN event_revision;
ALTER TABLE finance_write_requests DROP CONSTRAINT finance_write_requests_operation_check;
ALTER TABLE finance_write_requests ADD CONSTRAINT finance_write_requests_operation_check CHECK (operation IN (
    'account.create', 'event.create', 'transfer.create', 'opening_balance.confirm', 'event.void', 'event.delete'
));
COMMIT;
