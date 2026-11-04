BEGIN;
SET LOCAL lock_timeout='5s';
LOCK TABLE goals IN SHARE ROW EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM goals WHERE deleted_at IS NOT NULL) THEN
        RAISE EXCEPTION 'Refusing to make deleted Goals visible; use a reviewed forward correction';
    END IF;
END $$;
ALTER TABLE goals DROP COLUMN deleted_at;
COMMIT;
