-- +goose Up
ALTER TABLE transactions
    ADD COLUMN card_type text NULL
    CHECK (card_type IN ('credit_card', 'debit_card'));

-- +goose Down
ALTER TABLE transactions DROP COLUMN card_type;