-- +goose Up
ALTER TABLE transactions
    ADD COLUMN category_source text NULL
        CHECK (category_source IN ('user', 'jev')),
    ADD COLUMN category_confidence numeric NULL
        CHECK (category_confidence IS NULL OR (category_confidence >= 0 AND category_confidence <= 1)),
    ADD COLUMN category_model text NULL,
    ADD COLUMN category_categorized_at timestamptz NULL;

CREATE INDEX transactions_uncategorized_idx
    ON transactions (occurred_at DESC, id DESC)
    WHERE category_id IS NULL;

CREATE TABLE category_mappings (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    normalized_counterparty text NOT NULL CHECK (char_length(normalized_counterparty) > 0),
    kind text NOT NULL,
    direction text NOT NULL CHECK (direction IN ('debit', 'credit')),
    currency char(3) NOT NULL,
    category_id uuid NOT NULL REFERENCES categories(id),
    source text NOT NULL CHECK (source IN ('user', 'jev')),
    confidence numeric NULL CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    model text NULL,
    categorized_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (normalized_counterparty, kind, direction, currency)
);

CREATE INDEX category_mappings_category_id_idx ON category_mappings (category_id);

-- +goose Down
DROP TABLE category_mappings;
DROP INDEX transactions_uncategorized_idx;
ALTER TABLE transactions
    DROP COLUMN category_source,
    DROP COLUMN category_confidence,
    DROP COLUMN category_model,
    DROP COLUMN category_categorized_at;
