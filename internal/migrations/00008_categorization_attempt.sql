-- +goose Up
ALTER TABLE transactions
    ADD COLUMN category_attempted_at timestamptz NULL;

CREATE INDEX transactions_categorization_pending_idx
    ON transactions (occurred_at ASC, id ASC)
    WHERE category_id IS NULL AND category_attempted_at IS NULL;

-- +goose Down
DROP INDEX transactions_categorization_pending_idx;
ALTER TABLE transactions
    DROP COLUMN category_attempted_at;