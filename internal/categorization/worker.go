package categorization

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pakatagoh/finance/internal/storage"
)

type storageMappings struct{ store storage.CategorizationStore }

func (m storageMappings) Lookup(ctx context.Context, key MappingKey) (*Mapping, error) {
	row, err := m.store.GetCategoryMapping(ctx, key.Counterparty, key.Kind, key.Direction, key.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var slug string
	if err := m.store.Pool.QueryRow(ctx, `SELECT slug FROM categories WHERE id=$1`, row.CategoryID).Scan(&slug); err != nil {
		return nil, err
	}
	return &Mapping{Category: slug}, nil
}

// NewStorageMappingStore adapts persisted mappings to the service's lookup seam.
func NewStorageMappingStore(store storage.CategorizationStore) MappingStore {
	return storageMappings{store: store}
}

type BatchStore interface {
	EligibleUncategorizedTransactions(context.Context, int) ([]storage.CategorizationTransaction, error)
	ActiveCategories(context.Context) ([]storage.Category, error)
	UpsertCategoryMapping(context.Context, storage.CategoryMapping) (storage.CategoryMapping, error)
	ApplyCategoryProvenance(context.Context, string, storage.CategoryProvenance) error
	MarkCategorizationAttempted(context.Context, string) error
}

type batchService interface {
	Categorize(context.Context, Transaction) (Result, error)
}

// RunBatch processes a stable, bounded batch. One transaction failure is logged
// without sensitive data and never prevents subsequent transactions.
func RunBatch(ctx context.Context, store BatchStore, service batchService, model string, limit int, logger *slog.Logger) (processed, failed int, err error) {
	if limit <= 0 {
		limit = 100
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	categories, err := store.ActiveCategories(ctx)
	if err != nil {
		return 0, 0, err
	}
	bySlug := make(map[string]string, len(categories))
	for _, c := range categories {
		bySlug[c.Slug] = c.ID
	}
	candidates, err := store.EligibleUncategorizedTransactions(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, candidate := range candidates {
		tx := Transaction{Kind: candidate.Kind, Direction: candidate.Direction, Currency: candidate.Currency, Counterparty: candidate.Counterparty, NormalizedCounterparty: candidate.NormalizedCounterparty, AmountMinor: candidate.AmountMinor, Merchant: stringValue(candidate.Merchant), Payee: stringValue(candidate.Payee)}
		result, e := service.Categorize(ctx, tx)
		if result.Attempted {
			if markErr := store.MarkCategorizationAttempted(ctx, candidate.ID); markErr != nil {
				failed++
				logger.Error("categorization attempt marker failed", "transaction_id", candidate.ID)
				continue
			}
		}
		if e != nil {
			failed++
			logger.Error("categorization failed", "transaction_id", candidate.ID)
			continue
		}
		if result.Category == "" {
			processed++
			continue
		}
		categoryID, ok := bySlug[result.Category]
		if !ok {
			failed++
			logger.Error("categorization returned inactive category", "transaction_id", candidate.ID)
			continue
		}
		confidence := result.Confidence
		if result.Provenance == ProvenanceMapping {
			if e = store.ApplyCategoryProvenance(ctx, candidate.ID, storage.CategoryProvenance{CategoryID: categoryID, Source: storage.MappingSourceUser, Confidence: nil, Model: nil, CategorizedAt: time.Now().UTC()}); e != nil {
				failed++
				logger.Error("mapping provenance failed", "transaction_id", candidate.ID)
				continue
			}
			processed++
			continue
		}
		mapping, e := store.UpsertCategoryMapping(ctx, storage.CategoryMapping{NormalizedCounterparty: candidate.NormalizedCounterparty, Kind: candidate.Kind, Direction: candidate.Direction, Currency: candidate.Currency, CategoryID: categoryID, Source: storage.MappingSourceJev, Confidence: &confidence, Model: &model, CategorizedAt: time.Now().UTC()})
		if e != nil {
			failed++
			logger.Error("categorization mapping failed", "transaction_id", candidate.ID)
			continue
		}
		// A user mapping returned by the store remains authoritative.
		if mapping.Source == storage.MappingSourceUser {
			processed++
			continue
		}
		e = store.ApplyCategoryProvenance(ctx, candidate.ID, storage.CategoryProvenance{CategoryID: categoryID, Source: storage.MappingSourceJev, Confidence: &confidence, Model: &model, CategorizedAt: time.Now().UTC()})
		if e != nil {
			failed++
			logger.Error("categorization provenance failed", "transaction_id", candidate.ID)
			continue
		}
		processed++
	}
	return processed, failed, nil
}
func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
