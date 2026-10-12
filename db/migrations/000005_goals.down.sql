BEGIN;

-- Preserve user-entered Goals during rollback; use a corrective forward migration instead.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM goals) OR EXISTS (SELECT 1 FROM goal_checklist_items) THEN
        RAISE EXCEPTION 'goal data exists; restore a verified backup or use a corrective forward migration';
    END IF;
END $$;

DROP TABLE goal_checklist_items;
DROP TABLE goals;

COMMIT;
