BEGIN;

CREATE TABLE finance_accounts (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    account_type text NOT NULL CHECK (account_type IN ('cash', 'bank', 'ewallet', 'savings', 'credit_card', 'other')),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    opening_balance_minor bigint NOT NULL DEFAULT 0,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, id),
    UNIQUE (owner_id, id, currency)
);
CREATE INDEX finance_accounts_owner_idx ON finance_accounts (owner_id, created_at, id);

CREATE TABLE finance_categories (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    kind text NOT NULL CHECK (kind IN ('expense', 'income')),
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, id),
    UNIQUE (owner_id, id, kind)
);
CREATE INDEX finance_categories_owner_idx ON finance_categories (owner_id, kind, created_at, id);

CREATE TABLE financial_events (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES lifeos_users(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('expense', 'income', 'transfer')),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    occurred_at timestamptz NOT NULL,
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 1000),
    status text NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'voided')),
    voided_at timestamptz,
    void_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT financial_events_void_shape CHECK (
        (status = 'posted' AND voided_at IS NULL AND void_reason IS NULL) OR
        (status = 'voided' AND voided_at IS NOT NULL AND length(btrim(void_reason)) BETWEEN 1 AND 500)
    ),
    UNIQUE (owner_id, id),
    UNIQUE (owner_id, id, currency),
    UNIQUE (owner_id, id, currency, kind)
);
CREATE INDEX financial_events_owner_history_idx ON financial_events (owner_id, occurred_at DESC, id DESC);

CREATE TABLE account_movements (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL,
    event_id uuid NOT NULL,
    account_id uuid NOT NULL,
    amount_minor bigint NOT NULL CHECK (amount_minor <> 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_movements_event_owner_fk FOREIGN KEY (owner_id, event_id, currency)
        REFERENCES financial_events(owner_id, id, currency) ON DELETE RESTRICT,
    CONSTRAINT account_movements_account_owner_fk FOREIGN KEY (owner_id, account_id, currency)
        REFERENCES finance_accounts(owner_id, id, currency) ON DELETE RESTRICT,
    UNIQUE (owner_id, id),
    UNIQUE (owner_id, event_id, account_id)
);
CREATE INDEX account_movements_balance_idx ON account_movements (owner_id, account_id, event_id);

CREATE TABLE category_allocations (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL,
    event_id uuid NOT NULL,
    category_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('expense', 'income')),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT category_allocations_event_owner_fk FOREIGN KEY (owner_id, event_id, currency, kind)
        REFERENCES financial_events(owner_id, id, currency, kind) ON DELETE RESTRICT,
    CONSTRAINT category_allocations_category_owner_fk FOREIGN KEY (owner_id, category_id, kind)
        REFERENCES finance_categories(owner_id, id, kind) ON DELETE RESTRICT,
    UNIQUE (owner_id, id),
    UNIQUE (owner_id, event_id, category_id)
);
CREATE INDEX category_allocations_owner_event_idx ON category_allocations (owner_id, event_id);

COMMIT;
