package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSourceConflict = errors.New("source identity conflict")

type TransactionInput struct {
	SourceMailbox      string    `json:"source_mailbox"`
	GmailMessageID     string    `json:"gmail_message_id"`
	OccurredAt         time.Time `json:"occurred_at"`
	TimestampSource    string    `json:"timestamp_source"`
	SourceOccurredText string    `json:"source_occurred_text"`
	Bank               string    `json:"bank"`
	SourceType         string    `json:"source_type"`
	Kind               string    `json:"kind"`
	CardType           *string   `json:"card_type"`
	Direction          string    `json:"direction"`
	Currency           string    `json:"currency"`
	AmountMinor        int64     `json:"amount_minor"`
	CardSuffix         *string   `json:"card_suffix"`
	FromAccountSuffix  *string   `json:"from_account_suffix"`
	Payee              *string   `json:"payee"`
	Merchant           *string   `json:"merchant"`
}

type Transaction struct {
	ID string `json:"id"`
	TransactionInput
	CategoryID *string   `json:"category_id"`
	Notes      *string   `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TransactionStore atomically inserts source transactions and lists them.
type TransactionStore struct{ Pool *pgxpool.Pool }

func (s TransactionStore) ListTransactions(ctx context.Context, filter TransactionFilter, page int) (TransactionPage, error) {
	return ListTransactions(ctx, s.Pool, filter, page)
}

func (s TransactionStore) Ingest(ctx context.Context, in TransactionInput) (Transaction, bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Transaction{}, false, err
	}
	defer tx.Rollback(ctx)
	const cols = `id, source_mailbox, gmail_message_id, occurred_at, timestamp_source, source_occurred_text, bank, source_type, kind, card_type, direction, currency, amount_minor, card_suffix, from_account_suffix, payee, merchant, category_id, notes, created_at, updated_at`
	args := []any{in.SourceMailbox, in.GmailMessageID, in.OccurredAt, in.TimestampSource, in.SourceOccurredText, in.Bank, in.SourceType, in.Kind, in.CardType, in.Direction, in.Currency, in.AmountMinor, in.CardSuffix, in.FromAccountSuffix, in.Payee, in.Merchant}
	q := `INSERT INTO transactions (source_mailbox,gmail_message_id,occurred_at,timestamp_source,source_occurred_text,bank,source_type,kind,card_type,direction,currency,amount_minor,card_suffix,from_account_suffix,payee,merchant) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT (source_mailbox,gmail_message_id) DO NOTHING RETURNING ` + cols
	row := tx.QueryRow(ctx, q, args...)
	out, err := scanTransaction(row)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Transaction{}, false, err
		}
		return out, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, false, err
	}
	row = tx.QueryRow(ctx, `SELECT `+cols+` FROM transactions WHERE source_mailbox=$1 AND gmail_message_id=$2 FOR UPDATE`, in.SourceMailbox, in.GmailMessageID)
	out, err = scanTransaction(row)
	if err != nil {
		return Transaction{}, false, err
	}
	if !sameSource(out, in) {
		return Transaction{}, false, ErrSourceConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return Transaction{}, false, err
	}
	return out, true, nil
}

func sameSource(t Transaction, in TransactionInput) bool {
	a := t.TransactionInput
	return a.SourceMailbox == in.SourceMailbox && a.GmailMessageID == in.GmailMessageID && a.OccurredAt.Equal(in.OccurredAt) && a.TimestampSource == in.TimestampSource && a.SourceOccurredText == in.SourceOccurredText && a.Bank == in.Bank && a.SourceType == in.SourceType && a.Kind == in.Kind && eq(a.CardType, in.CardType) && a.Direction == in.Direction && a.Currency == in.Currency && a.AmountMinor == in.AmountMinor && eq(a.CardSuffix, in.CardSuffix) && eq(a.FromAccountSuffix, in.FromAccountSuffix) && eq(a.Payee, in.Payee) && eq(a.Merchant, in.Merchant)
}
func eq(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type rowScanner interface{ Scan(...any) error }

func scanTransaction(r rowScanner) (Transaction, error) {
	var t Transaction
	err := r.Scan(&t.ID, &t.SourceMailbox, &t.GmailMessageID, &t.OccurredAt, &t.TimestampSource, &t.SourceOccurredText, &t.Bank, &t.SourceType, &t.Kind, &t.CardType, &t.Direction, &t.Currency, &t.AmountMinor, &t.CardSuffix, &t.FromAccountSuffix, &t.Payee, &t.Merchant, &t.CategoryID, &t.Notes, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}
