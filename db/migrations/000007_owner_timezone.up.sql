BEGIN;

ALTER TABLE lifeos_users ADD COLUMN time_zone text
    CHECK (time_zone IS NULL OR (length(time_zone) BETWEEN 3 AND 100 AND time_zone = btrim(time_zone)));

COMMIT;
