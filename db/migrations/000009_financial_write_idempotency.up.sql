CREATE TABLE finance_write_requests (
    owner_id uuid NOT NULL REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    request_key uuid NOT NULL,
    operation text NOT NULL CHECK (operation IN (
        'account.create', 'event.create', 'transfer.create',
        'opening_balance.confirm', 'event.void', 'event.delete'
    )),
    fingerprint_version smallint NOT NULL CHECK (fingerprint_version > 0),
    request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (owner_id, request_key)
);
