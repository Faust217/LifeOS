BEGIN;

-- Rollback is intentionally unavailable once identity data exists.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM lifeos_users) OR EXISTS (SELECT 1 FROM invitations) THEN
        RAISE EXCEPTION 'identity data exists; restore a verified backup or use a corrective forward migration';
    END IF;
END $$;

DROP TABLE invitations;
DROP TABLE google_identities;
DROP TABLE lifeos_users;

COMMIT;
