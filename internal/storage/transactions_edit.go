package storage

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrTransactionNotFound = errors.New("transaction not found")
var ErrInvalidCategory = errors.New("category is not active")

type Category struct {
	ID   string
	Slug string
	Name string
}

type TransactionReader interface {
	GetTransaction(context.Context, string) (Transaction, error)
}

type TransactionEditor interface {
	UpdateEnrichment(context.Context, string, *string, *string) (Transaction, error)
}

func (s TransactionStore) GetTransaction(ctx context.Context, id string) (Transaction, error) {
	const q = `SELECT id, source_mailbox, gmail_message_id, occurred_at, timestamp_source, source_occurred_text, bank, source_type, kind, direction, currency, amount_minor, card_suffix, from_account_suffix, payee, merchant, category_id, notes, created_at, updated_at FROM transactions WHERE id=$1`
	out, err := scanTransaction(s.Pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, ErrTransactionNotFound
	}
	return out, err
}

// UpdateEnrichment is the only transaction mutation path exposed to the web UI.
// Source/imported columns are deliberately absent from its arguments and query.
func (s TransactionStore) UpdateEnrichment(ctx context.Context, id string, categoryID, notes *string) (Transaction, error) {
	if categoryID != nil {
		if _, err := uuid.Parse(*categoryID); err != nil {
			return Transaction{}, ErrInvalidCategory
		}
	}
	const q = `UPDATE transactions AS t SET category_id=$2, notes=$3, updated_at=now()
        WHERE t.id=$1 AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM categories c WHERE c.id=$2::uuid AND c.archived_at IS NULL))
        RETURNING id, source_mailbox, gmail_message_id, occurred_at, timestamp_source, source_occurred_text, bank, source_type, kind, direction, currency, amount_minor, card_suffix, from_account_suffix, payee, merchant, category_id, notes, created_at, updated_at`
	out, err := scanTransaction(s.Pool.QueryRow(ctx, q, id, categoryID, notes))
	if err == nil {
		return out, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if e := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transactions WHERE id=$1)`, id).Scan(&exists); e == nil && exists {
			return Transaction{}, ErrInvalidCategory
		}
		return Transaction{}, ErrTransactionNotFound
	}
	return Transaction{}, err
}

func (s TransactionStore) ActiveCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, slug, name FROM categories WHERE archived_at IS NULL ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
