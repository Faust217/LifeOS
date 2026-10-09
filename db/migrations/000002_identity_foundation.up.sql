BEGIN;

CREATE TABLE lifeos_users (
    id uuid PRIMARY KEY,
    email text NOT NULL CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 320 AND position('@' IN email) > 1),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    is_owner_admin boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX lifeos_users_one_owner_admin ON lifeos_users (is_owner_admin) WHERE is_owner_admin;

CREATE TABLE google_identities (
    user_id uuid PRIMARY KEY REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    google_sub text NOT NULL UNIQUE CHECK (length(google_sub) BETWEEN 1 AND 255 AND google_sub = btrim(google_sub)),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invitations (
    id uuid PRIMARY KEY,
    email text NOT NULL CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 320 AND position('@' IN email) > 1),
    issued_by uuid REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    owner_bootstrap boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL,
    accepted_by uuid UNIQUE REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    accepted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT invitations_issuer_shape CHECK ((owner_bootstrap AND issued_by IS NULL) OR (NOT owner_bootstrap AND issued_by IS NOT NULL)),
    CONSTRAINT invitations_outcome_shape CHECK (
        (accepted_by IS NULL AND accepted_at IS NULL AND (revoked_at IS NULL OR revoked_at >= created_at)) OR
        (accepted_by IS NOT NULL AND accepted_at IS NOT NULL AND revoked_at IS NULL AND accepted_at >= created_at AND accepted_at < expires_at)
    ),
    CONSTRAINT invitations_expiry CHECK (expires_at > created_at)
);

-- One bootstrap invitation may ever exist. Subsequent invites require the admitted owner administrator.
CREATE UNIQUE INDEX invitations_one_bootstrap ON invitations (owner_bootstrap) WHERE owner_bootstrap;
-- A pending or expired invite must be revoked before another is issued for the same address.
CREATE UNIQUE INDEX invitations_one_unaccepted_email ON invitations (email) WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX invitations_issued_by_idx ON invitations (issued_by) WHERE issued_by IS NOT NULL;
CREATE INDEX invitations_pending_expiry_idx ON invitations (expires_at) WHERE accepted_at IS NULL AND revoked_at IS NULL;

COMMIT;
