BEGIN;

ALTER TABLE finance_accounts
    ADD COLUMN opening_balance_at timestamptz,
    ADD COLUMN opening_balance_confirmed_at timestamptz,
    ADD COLUMN balance_revision bigint NOT NULL DEFAULT 0 CHECK (balance_revision >= 0),
    ADD CONSTRAINT finance_accounts_opening_instant_shape CHECK (
        (opening_balance_at IS NULL AND opening_balance_confirmed_at IS NULL) OR
        (opening_balance_at IS NOT NULL AND opening_balance_confirmed_at IS NOT NULL
         AND opening_balance_at <= opening_balance_confirmed_at)
    );

-- Existing rows deliberately remain undated and keep their original opening value.
CREATE TABLE finance_opening_confirmations (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL,
    account_id uuid NOT NULL,
    previous_opening_balance_minor bigint NOT NULL,
    previous_balance_minor bigint NOT NULL,
    opening_balance_at timestamptz NOT NULL,
    opening_balance_minor bigint NOT NULL,
    suggested_opening_balance_minor numeric NOT NULL,
    resulting_balance_minor bigint NOT NULL,
    assertion_choice text NOT NULL CHECK (assertion_choice IN ('preserve', 'known_balance')),
    previous_revision bigint NOT NULL CHECK (previous_revision >= 0),
    confirmed_at timestamptz NOT NULL,
    CONSTRAINT finance_opening_confirmation_owner_fk FOREIGN KEY (owner_id, account_id)
        REFERENCES finance_accounts(owner_id, id) ON DELETE RESTRICT,
    UNIQUE (owner_id, account_id),
    CHECK (opening_balance_at <= confirmed_at)
);

COMMIT;
