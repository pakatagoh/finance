-- +goose Up
-- Transitional constraint: card_purchase remains until the Sheet-backed
-- historical backfill is completed.
ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('card_purchase', 'credit_card', 'debit_card', 'paynow', 'funds_transfer', 'incoming_transfer', 'reversal'));

-- +goose Down
ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('card_purchase', 'paynow', 'funds_transfer', 'incoming_transfer', 'reversal'));