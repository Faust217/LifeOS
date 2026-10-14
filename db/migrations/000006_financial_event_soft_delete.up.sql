BEGIN;

ALTER TABLE financial_events ADD COLUMN deleted_at timestamptz;

COMMIT;
