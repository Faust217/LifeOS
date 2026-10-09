BEGIN;

-- Authentication history and live sessions must not be silently deleted.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM lifeos_sessions) OR EXISTS (SELECT 1 FROM oidc_attempts) THEN
        RAISE EXCEPTION 'authentication data exists; use a reviewed forward migration or verified backup';
    END IF;
END $$;

DROP TABLE lifeos_sessions;
DROP TABLE oidc_attempts;

COMMIT;
