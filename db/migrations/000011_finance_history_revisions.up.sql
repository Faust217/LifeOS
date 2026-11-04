BEGIN;
SET LOCAL lock_timeout='5s';
-- Freeze old in-flight DML before taking the owner baseline or installing the
-- writer fence. Reads remain allowed; a busy upgrade fails rather than drifting.
LOCK TABLE account_movements,category_allocations,finance_accounts,finance_categories,
    finance_opening_confirmations,finance_write_requests,financial_event_revisions,financial_events
    IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE finance_history_revisions (
    owner_id uuid PRIMARY KEY REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    epoch uuid NOT NULL,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);
INSERT INTO finance_history_revisions(owner_id,epoch,revision)
SELECT owner_id,gen_random_uuid(),0 FROM (
    SELECT owner_id FROM finance_accounts UNION SELECT owner_id FROM finance_categories
    UNION SELECT owner_id FROM financial_events
) owners;

-- Fence old processes: readiness alone does not stop an already-running v10 writer.
-- This marker is a compatibility guard, not authorization against a database operator.
CREATE FUNCTION require_finance_writer_v11() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('lifeos.finance_writer_version', true) IS DISTINCT FROM '11' THEN
        RAISE EXCEPTION 'Finance writer upgrade required' USING ERRCODE='55000';
    END IF;
    RETURN NULL;
END;
$$;
DO $$ DECLARE relation_name text; BEGIN
    FOREACH relation_name IN ARRAY ARRAY['finance_accounts','finance_categories','financial_events',
        'account_movements','category_allocations','finance_opening_confirmations',
        'financial_event_revisions','finance_write_requests','finance_history_revisions'] LOOP
        EXECUTE format('CREATE TRIGGER finance_writer_v11 BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION require_finance_writer_v11()',relation_name);
    END LOOP;
END $$;
COMMIT;
