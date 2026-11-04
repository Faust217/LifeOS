BEGIN;
SET LOCAL lock_timeout='5s';
ALTER TABLE goals ADD COLUMN deleted_at timestamptz;
COMMIT;
