package storage

import (
	"context"
	"errors"
	"strings"

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
	UpdateEnrichment(context.Context, string, *string, *string, bool) (Transaction, error)
}

func (s TransactionStore) GetTransaction(ctx context.Context, id string) (Transaction, error) {
	const q = `SELECT id, source_mailbox, gmail_message_id, occurred_at, timestamp_source, source_occurred_text, bank, source_type, kind, direction, currency, amount_minor, card_suffix, from_account_suffix, payee, merchant, category_id, COALESCE(category_source, ''), category_confidence::double precision, category_model, notes, created_at, updated_at FROM transactions WHERE id=$1`
	out, err := scanTransaction(s.Pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, ErrTransactionNotFound
	}
	return out, err
}

// UpdateEnrichment is the only transaction mutation path exposed to the web UI.
// Source/imported columns are deliberately absent from its arguments and query.
func (s TransactionStore) UpdateEnrichment(ctx context.Context, id string, categoryID, notes *string, applyToMatching bool) (Transaction, error) {
	if categoryID != nil {
		if _, err := uuid.Parse(*categoryID); err != nil {
			return Transaction{}, ErrInvalidCategory
		}
	}
	dbtx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Transaction{}, err
	}
	defer dbtx.Rollback(ctx)
	const q = `UPDATE transactions AS t SET category_id=$2, category_source=CASE WHEN $2::uuid IS NULL THEN NULL ELSE 'user' END, category_confidence=NULL, category_model=NULL, category_categorized_at=CASE WHEN $2::uuid IS NULL THEN NULL ELSE now() END, notes=$3, updated_at=now()
        WHERE t.id=$1 AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM categories c WHERE c.id=$2::uuid AND c.archived_at IS NULL))
        RETURNING id, source_mailbox, gmail_message_id, occurred_at, timestamp_source, source_occurred_text, bank, source_type, kind, direction, currency, amount_minor, card_suffix, from_account_suffix, payee, merchant, category_id, COALESCE(category_source, ''), category_confidence::double precision, category_model, notes, created_at, updated_at`
	out, err := scanTransaction(dbtx.QueryRow(ctx, q, id, categoryID, notes))
	if err == nil {
		if err := s.persistUserMapping(ctx, dbtx, out, categoryID); err != nil {
			return Transaction{}, err
		}
		if applyToMatching && categoryID != nil {
			if err := s.applyCategoryToMatching(ctx, dbtx, out, *categoryID); err != nil {
				return Transaction{}, err
			}
		}
		if err := dbtx.Commit(ctx); err != nil {
			return Transaction{}, err
		}
		return out, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if e := dbtx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transactions WHERE id=$1)`, id).Scan(&exists); e == nil && exists {
			return Transaction{}, ErrInvalidCategory
		}
		return Transaction{}, ErrTransactionNotFound
	}
	return Transaction{}, err
}

func (s TransactionStore) applyCategoryToMatching(ctx context.Context, dbtx pgx.Tx, tx Transaction, categoryID string) error {
	counterparty := transactionCounterparty(tx)
	normalized := NormalizeCounterparty(counterparty)
	if normalized == "" {
		return nil
	}
	_, err := dbtx.Exec(ctx, `UPDATE transactions SET category_id=$1, category_source='user', category_confidence=NULL, category_model=NULL, category_categorized_at=now(), updated_at=now()
        WHERE lower(trim(regexp_replace(COALESCE(NULLIF(merchant, ''), NULLIF(payee, '')), '[^[:alnum:]]+', ' ', 'g'))) = $2
          AND kind=$3 AND direction=$4 AND currency=$5`, categoryID, normalized, tx.Kind, tx.Direction, tx.Currency)
	return err
}

func (s TransactionStore) persistUserMapping(ctx context.Context, dbtx pgx.Tx, tx Transaction, categoryID *string) error {
	counterparty := transactionCounterparty(tx)
	normalized := NormalizeCounterparty(counterparty)
	if normalized == "" {
		return nil
	}
	if categoryID == nil {
		_, err := dbtx.Exec(ctx, `DELETE FROM category_mappings WHERE normalized_counterparty=$1 AND kind=$2 AND direction=$3 AND currency=$4 AND source='user'`, normalized, tx.Kind, tx.Direction, tx.Currency)
		return err
	}
	_, err := dbtx.Exec(ctx, `INSERT INTO category_mappings (normalized_counterparty, kind, direction, currency, category_id, source, confidence, model) VALUES ($1,$2,$3,$4,$5,'user',NULL,NULL) ON CONFLICT (normalized_counterparty,kind,direction,currency) DO UPDATE SET category_id=EXCLUDED.category_id, source='user', confidence=NULL, model=NULL, categorized_at=now(), updated_at=now()`, normalized, tx.Kind, tx.Direction, tx.Currency, *categoryID)
	return err
}

func transactionCounterparty(tx Transaction) string {
	if tx.Merchant != nil && strings.TrimSpace(*tx.Merchant) != "" {
		return *tx.Merchant
	}
	if tx.Payee != nil {
		return *tx.Payee
	}
	return ""
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
