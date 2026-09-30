-- Foundation baseline rollback: intentionally changes no LifeOS domain schema.
BEGIN;
SELECT 1;
COMMIT;
