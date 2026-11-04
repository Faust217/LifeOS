BEGIN;
SET LOCAL lock_timeout='5s';
LOCK TABLE account_movements,category_allocations,finance_accounts,finance_categories,
    finance_opening_confirmations,finance_write_requests,financial_event_revisions,financial_events,
    finance_history_revisions IN SHARE ROW EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM finance_history_revisions WHERE revision > 0) THEN
        RAISE EXCEPTION 'Refusing to discard active History versions; use a reviewed forward correction';
    END IF;
END $$;
DO $$ DECLARE relation_name text; BEGIN
    FOREACH relation_name IN ARRAY ARRAY['finance_accounts','finance_categories','financial_events',
        'account_movements','category_allocations','finance_opening_confirmations',
        'financial_event_revisions','finance_write_requests','finance_history_revisions'] LOOP
        EXECUTE format('DROP TRIGGER finance_writer_v11 ON %I',relation_name);
    END LOOP;
END $$;
DROP FUNCTION require_finance_writer_v11();
DROP TABLE finance_history_revisions;
COMMIT;
