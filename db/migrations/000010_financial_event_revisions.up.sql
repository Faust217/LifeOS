BEGIN;

ALTER TABLE financial_events ADD COLUMN event_revision bigint NOT NULL DEFAULT 1 CHECK (event_revision > 0);
ALTER TABLE finance_write_requests DROP CONSTRAINT finance_write_requests_operation_check;
ALTER TABLE finance_write_requests ADD CONSTRAINT finance_write_requests_operation_check CHECK (operation IN (
    'account.create', 'event.create', 'transfer.create', 'opening_balance.confirm', 'event.void', 'event.delete', 'event.edit'
));

CREATE TABLE financial_event_revisions (
    owner_id uuid NOT NULL,
    event_id uuid NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    action text NOT NULL CHECK (action IN ('legacy_baseline','create','edit','void','delete')),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    request_key uuid,
    snapshot_schema_version smallint NOT NULL DEFAULT 1 CHECK (snapshot_schema_version = 1),
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    PRIMARY KEY (owner_id,event_id,revision),
    FOREIGN KEY (owner_id,event_id) REFERENCES financial_events(owner_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (owner_id,request_key) REFERENCES finance_write_requests(owner_id,request_key) ON DELETE RESTRICT,
    CHECK ((action = 'legacy_baseline') = (request_key IS NULL))
);

-- A complete resulting-state snapshot. Timestamps and money are exact strings.
CREATE FUNCTION capture_financial_event_revision(p_owner uuid, p_event uuid, p_action text, p_key uuid)
RETURNS void LANGUAGE sql AS $$
    INSERT INTO financial_event_revisions(owner_id,event_id,revision,action,request_key,snapshot)
    SELECT e.owner_id,e.id,e.event_revision,p_action,p_key,
      jsonb_build_object('schema','lifeos-financial-revision-v1','event',
        jsonb_build_object('id',e.id,'kind',e.kind,'amount_minor',e.amount_minor::text,'currency',e.currency,
          'occurred_at',to_char(e.occurred_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
          'created_at',to_char(e.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
          'note',e.note,'status',e.status,'void_reason',e.void_reason,
          'voided_at',to_char(e.voided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
          'deleted_at',to_char(e.deleted_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')),
        'movements',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'account_id',m.account_id,
          'amount_minor',m.amount_minor::text,'currency',m.currency,
          'created_at',to_char(m.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) ORDER BY m.id)
          FROM account_movements m WHERE m.owner_id=e.owner_id AND m.event_id=e.id),'[]'::jsonb),
        'allocations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',a.id,'category_id',a.category_id,'kind',a.kind,
          'amount_minor',a.amount_minor::text,'currency',a.currency,
          'created_at',to_char(a.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) ORDER BY a.id)
          FROM category_allocations a WHERE a.owner_id=e.owner_id AND a.event_id=e.id),'[]'::jsonb))
    FROM financial_events e WHERE e.owner_id=p_owner AND e.id=p_event;
$$;

-- Captured at upgrade, without claiming earlier unrecorded revisions.
SELECT capture_financial_event_revision(owner_id,id,'legacy_baseline',NULL) FROM financial_events;

CREATE FUNCTION reject_financial_revision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Financial revision snapshots are append-only';
END;
$$;
CREATE TRIGGER financial_revisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON financial_event_revisions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_financial_revision_mutation();

COMMIT;
