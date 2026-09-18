package storage

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MappingSourceUser = "user"
	MappingSourceJev  = "jev"
)

var ErrInvalidMappingSource = errors.New("invalid category mapping source")

// NormalizeCounterparty produces the stable lookup key shared by transactions
// and reusable category mappings.
func NormalizeCounterparty(counterparty string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(counterparty))), " ")
}

// CanReplaceMapping reports whether incoming may replace existing. User choices
// always win over Jev results, while same-source rows may be refreshed.
func CanReplaceMapping(existing, incoming string) bool {
	return existing != MappingSourceUser || incoming == MappingSourceUser
}

type CategoryProvenance struct {
	CategoryID    string
	Source        string
	Confidence    *float64
	Model         *string
	CategorizedAt time.Time
}

func (p CategoryProvenance) Equal(other CategoryProvenance) bool {
	return p.CategoryID == other.CategoryID && p.Source == other.Source && equalFloat(p.Confidence, other.Confidence) && equalString(p.Model, other.Model) && p.CategorizedAt.Equal(other.CategorizedAt)
}

func equalFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func equalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type CategorizationTransaction struct {
	ID                     string
	Counterparty           string
	NormalizedCounterparty string
	OccurredAt             time.Time
	Kind                   string
	Direction              string
	Currency               string
}

type CategoryMapping struct {
	ID                                  string
	NormalizedCounterparty              string
	Kind, Direction, Currency           string
	CategoryID                          string
	Source                              string
	Confidence                          *float64
	Model                               *string
	CategorizedAt, CreatedAt, UpdatedAt time.Time
}

type CategorizationStore struct{ Pool *pgxpool.Pool }

// EligibleUncategorizedTransactions returns rows with a usable merchant/payee
// and no category. All transaction kinds are intentionally included.
func (s CategorizationStore) EligibleUncategorizedTransactions(ctx context.Context, limit int) ([]CategorizationTransaction, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `SELECT t.id, COALESCE(NULLIF(t.merchant, ''), NULLIF(t.payee, '')), lower(trim(regexp_replace(COALESCE(NULLIF(t.merchant, ''), NULLIF(t.payee, '')), '\s+', ' ', 'g'))), t.occurred_at, t.kind, t.direction, t.currency FROM transactions t WHERE t.category_id IS NULL AND NULLIF(trim(COALESCE(NULLIF(t.merchant, ''), NULLIF(t.payee, ''))), '') IS NOT NULL ORDER BY t.occurred_at ASC, t.id ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategorizationTransaction
	for rows.Next() {
		var item CategorizationTransaction
		if err := rows.Scan(&item.ID, &item.Counterparty, &item.NormalizedCounterparty, &item.OccurredAt, &item.Kind, &item.Direction, &item.Currency); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanCategoryMapping(row rowScanner) (CategoryMapping, error) {
	var m CategoryMapping
	err := row.Scan(&m.ID, &m.NormalizedCounterparty, &m.Kind, &m.Direction, &m.Currency, &m.CategoryID, &m.Source, &m.Confidence, &m.Model, &m.CategorizedAt, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

const categoryMappingColumns = `id, normalized_counterparty, kind, direction, currency, category_id, source, confidence, model, categorized_at, created_at, updated_at`

func (s CategorizationStore) GetCategoryMapping(ctx context.Context, normalizedCounterparty, kind, direction, currency string) (CategoryMapping, error) {
	return scanCategoryMapping(s.Pool.QueryRow(ctx, `SELECT `+categoryMappingColumns+` FROM category_mappings WHERE normalized_counterparty=$1 AND kind=$2 AND direction=$3 AND currency=$4`, NormalizeCounterparty(normalizedCounterparty), kind, direction, currency))
}

// UpsertCategoryMapping refreshes Jev mappings but never lets them overwrite a
// user mapping. A user mapping may deliberately replace a Jev mapping.
func (s CategorizationStore) UpsertCategoryMapping(ctx context.Context, mapping CategoryMapping) (CategoryMapping, error) {
	if mapping.Source != MappingSourceUser && mapping.Source != MappingSourceJev {
		return CategoryMapping{}, ErrInvalidMappingSource
	}
	normalized := NormalizeCounterparty(mapping.NormalizedCounterparty)
	if normalized == "" {
		return CategoryMapping{}, errors.New("normalized counterparty is required")
	}
	q := `INSERT INTO category_mappings (normalized_counterparty, kind, direction, currency, category_id, source, confidence, model, categorized_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9,now())) ON CONFLICT (normalized_counterparty,kind,direction,currency) DO UPDATE SET category_id=EXCLUDED.category_id, source=EXCLUDED.source, confidence=EXCLUDED.confidence, model=EXCLUDED.model, categorized_at=EXCLUDED.categorized_at, updated_at=now() WHERE category_mappings.source <> 'user' OR EXCLUDED.source = 'user' RETURNING ` + categoryMappingColumns
	out, err := scanCategoryMapping(s.Pool.QueryRow(ctx, q, normalized, mapping.Kind, mapping.Direction, mapping.Currency, mapping.CategoryID, mapping.Source, mapping.Confidence, mapping.Model, mapping.CategorizedAt))
	if err == nil {
		return out, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// A lower-priority Jev write is an intentional no-op; return the
		// authoritative existing mapping to make retries and callers simple.
		return s.GetCategoryMapping(ctx, normalized, mapping.Kind, mapping.Direction, mapping.Currency)
	}
	return CategoryMapping{}, err
}

// ApplyCategoryProvenance sets category and provenance atomically and is safe to
// retry. Jev cannot replace a user-confirmed category.
func (s CategorizationStore) ApplyCategoryProvenance(ctx context.Context, transactionID string, provenance CategoryProvenance) error {
	if provenance.Source != MappingSourceUser && provenance.Source != MappingSourceJev {
		return ErrInvalidMappingSource
	}
	_, err := s.Pool.Exec(ctx, `UPDATE transactions SET category_id=$2, category_source=$3, category_confidence=$4, category_model=$5, category_categorized_at=$6, updated_at=now() WHERE id=$1 AND (category_source IS DISTINCT FROM 'user' OR $3='user') AND (category_id IS DISTINCT FROM $2 OR category_source IS DISTINCT FROM $3 OR category_confidence IS DISTINCT FROM $4 OR category_model IS DISTINCT FROM $5 OR category_categorized_at IS DISTINCT FROM $6)`, transactionID, provenance.CategoryID, provenance.Source, provenance.Confidence, provenance.Model, provenance.CategorizedAt)
	return err
}
