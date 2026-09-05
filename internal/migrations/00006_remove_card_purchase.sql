-- +goose Up
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transactions WHERE kind = 'card_purchase') THEN
        RAISE EXCEPTION 'cannot remove card_purchase: historical rows remain';
    END IF;
END
$$;

ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('credit_card', 'debit_card', 'paynow', 'funds_transfer', 'incoming_transfer', 'reversal'));
ALTER TABLE transactions DROP COLUMN card_type;

-- +goose Down
ALTER TABLE transactions ADD COLUMN card_type text NULL
    CHECK (card_type IN ('credit_card', 'debit_card'));
ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('card_purchase', 'credit_card', 'debit_card', 'paynow', 'funds_transfer', 'incoming_transfer', 'reversal'));