-- +goose Up
CREATE TABLE transactions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    source_mailbox text NOT NULL,
    gmail_message_id text NOT NULL,
    occurred_at timestamptz NOT NULL,
    timestamp_source text NOT NULL CHECK (timestamp_source IN ('transaction', 'email_received', 'inferred')),
    source_occurred_text text NOT NULL,
    bank text NOT NULL,
    source_type text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('card_purchase', 'paynow', 'funds_transfer', 'incoming_transfer', 'reversal')),
    direction text NOT NULL CHECK (direction IN ('debit', 'credit')),
    card_suffix text,
    from_account_suffix text,
    payee text,
    merchant text,
    currency char(3) NOT NULL,
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    category_id uuid NULL REFERENCES categories(id),
    notes text NULL CHECK (notes IS NULL OR char_length(notes) <= 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_mailbox, gmail_message_id)
);

CREATE INDEX transactions_occurred_at_id_idx ON transactions (occurred_at DESC, id DESC);
CREATE INDEX transactions_bank_idx ON transactions (bank);
CREATE INDEX transactions_source_type_idx ON transactions (source_type);
CREATE INDEX transactions_category_id_idx ON transactions (category_id);

-- +goose Down
DROP TABLE transactions;
