BEGIN;

CREATE TABLE oidc_attempts (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
    nonce_hash bytea NOT NULL CHECK (octet_length(nonce_hash) = 32),
    pkce_verifier text NOT NULL CHECK (length(pkce_verifier) BETWEEN 43 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    CONSTRAINT oidc_attempt_expiry CHECK (expires_at > created_at),
    CONSTRAINT oidc_attempt_consumed CHECK (consumed_at IS NULL OR consumed_at >= created_at)
);

CREATE INDEX oidc_attempts_expiry_idx ON oidc_attempts (expires_at);

CREATE TABLE lifeos_sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT lifeos_session_expiry CHECK (expires_at > created_at),
    CONSTRAINT lifeos_session_revoked CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX lifeos_sessions_user_idx ON lifeos_sessions (user_id);
CREATE INDEX lifeos_sessions_expiry_idx ON lifeos_sessions (expires_at) WHERE revoked_at IS NULL;

COMMIT;
