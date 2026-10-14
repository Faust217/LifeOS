BEGIN;

-- A saved preference must not be silently discarded during a rollback.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM lifeos_users WHERE time_zone IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot remove owner timezone while preferences exist';
    END IF;
END $$;

ALTER TABLE lifeos_users DROP COLUMN time_zone;

COMMIT;
