BEGIN;

CREATE TABLE goals (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    title text NOT NULL CHECK (title = btrim(title) AND length(title) BETWEEN 1 AND 160),
    completed_at timestamptz,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, id)
);
CREATE INDEX goals_owner_idx ON goals (owner_id, created_at DESC, id DESC);

CREATE TABLE goal_checklist_items (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL,
    goal_id uuid NOT NULL,
    title text NOT NULL CHECK (title = btrim(title) AND length(title) BETWEEN 1 AND 160),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT goal_checklist_items_goal_owner_fk FOREIGN KEY (owner_id, goal_id)
        REFERENCES goals(owner_id, id) ON DELETE RESTRICT,
    UNIQUE (owner_id, id)
);
CREATE INDEX goal_checklist_items_goal_idx ON goal_checklist_items (owner_id, goal_id, created_at, id);

COMMIT;
