package categorization

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/pakatagoh/finance/internal/storage"
)

type batchStoreFake struct {
	categories []storage.Category
	candidates []storage.CategorizationTransaction
	limit      int
	upserts    []storage.CategoryMapping
	provenance []storage.CategoryProvenance
	failUpsert map[string]bool
}

func (s *batchStoreFake) EligibleUncategorizedTransactions(_ context.Context, limit int) ([]storage.CategorizationTransaction, error) {
	s.limit = limit
	return s.candidates, nil
}
func (s *batchStoreFake) ActiveCategories(context.Context) ([]storage.Category, error) {
	return s.categories, nil
}
func (s *batchStoreFake) UpsertCategoryMapping(_ context.Context, m storage.CategoryMapping) (storage.CategoryMapping, error) {
	s.upserts = append(s.upserts, m)
	if s.failUpsert[m.NormalizedCounterparty] {
		return storage.CategoryMapping{}, errors.New("upsert failed")
	}
	return m, nil
}
func (s *batchStoreFake) ApplyCategoryProvenance(_ context.Context, id string, p storage.CategoryProvenance) error {
	s.provenance = append(s.provenance, p)
	if id == "fail-apply" {
		return errors.New("apply failed")
	}
	return nil
}

type batchServiceFake struct {
	results map[string]Result
	errors  map[string]error
}

func (s batchServiceFake) Categorize(_ context.Context, tx Transaction) (Result, error) {
	if err := s.errors[tx.Counterparty]; err != nil {
		return Result{}, err
	}
	return s.results[tx.Counterparty], nil
}

func TestRunBatchMapsActiveCategoryContinuesFailuresAndPropagatesLimit(t *testing.T) {
	store := &batchStoreFake{categories: []storage.Category{{ID: "cat-dining", Slug: "dining"}}, candidates: []storage.CategorizationTransaction{
		{ID: "ok", Counterparty: "Cafe", NormalizedCounterparty: "cafe", Kind: "debit_card", Direction: "debit", Currency: "GBP"},
		{ID: "bad", Counterparty: "Broken", NormalizedCounterparty: "broken", Kind: "debit_card", Direction: "debit", Currency: "GBP"},
		{ID: "noop", Counterparty: "Low", NormalizedCounterparty: "low", Kind: "debit_card", Direction: "debit", Currency: "GBP"},
	}}
	service := batchServiceFake{results: map[string]Result{"Cafe": {Category: "dining", Confidence: .9}, "Low": {}}, errors: map[string]error{"Broken": errors.New("provider failed: jev-secret-token")}}
	var logs bytes.Buffer
	processed, failed, err := RunBatch(context.Background(), store, service, "jev-test", 7, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 || failed != 1 || store.limit != 7 {
		t.Fatalf("processed=%d failed=%d limit=%d", processed, failed, store.limit)
	}
	if len(store.provenance) != 1 || store.provenance[0].CategoryID != "cat-dining" {
		t.Fatalf("provenance=%#v", store.provenance)
	}
}

func TestRunBatchHonorsUserMappingAndDoesNotLogSecrets(t *testing.T) {
	const secret = "jev-secret-token"
	store := &batchStoreFake{categories: []storage.Category{{ID: "cat-food", Slug: "groceries"}}, candidates: []storage.CategorizationTransaction{{ID: "u", Counterparty: "Shop", NormalizedCounterparty: "shop", Kind: "debit_card", Direction: "debit", Currency: "GBP"}}}
	service := batchServiceFake{results: map[string]Result{"Shop": {Category: "groceries", Confidence: 1, Provenance: ProvenanceMapping}}, errors: map[string]error{}}
	var logs bytes.Buffer
	_, _, err := RunBatch(context.Background(), store, service, "jev-test", 1, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.upserts) != 0 || len(store.provenance) != 1 || store.provenance[0].Source != storage.MappingSourceUser {
		t.Fatalf("user mapping writes: upserts=%#v provenance=%#v", store.upserts, store.provenance)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("logs contain secret")
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("logs contain secret: %s", logs.String())
	}
}
