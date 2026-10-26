DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM finance_write_requests) THEN
        RAISE EXCEPTION 'Cannot discard committed financial request keys; use a reviewed forward correction';
    END IF;
END $$;
DROP TABLE finance_write_requests;
