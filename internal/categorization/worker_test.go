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
	attempted  []string
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

func (s *batchStoreFake) MarkCategorizationAttempted(_ context.Context, id string) error {
	s.attempted = append(s.attempted, id)
	return nil
}

type batchServiceFake struct {
	results map[string]Result
	errors  map[string]error
}

func (s batchServiceFake) Categorize(_ context.Context, tx Transaction) (Result, error) {
	if err := s.errors[tx.Counterparty]; err != nil {
		return Result{Attempted: true}, err
	}
	return s.results[tx.Counterparty], nil
}

func TestRunBatchMapsActiveCategoryContinuesFailuresAndPropagatesLimit(t *testing.T) {
	store := &batchStoreFake{categories: []storage.Category{{ID: "cat-dining", Slug: "dining"}}, candidates: []storage.CategorizationTransaction{
		{ID: "ok", Counterparty: "Cafe", NormalizedCounterparty: "cafe", Kind: "debit_card", Direction: "debit", Currency: "GBP"},
		{ID: "bad", Counterparty: "Broken", NormalizedCounterparty: "broken", Kind: "debit_card", Direction: "debit", Currency: "GBP"},
		{ID: "noop", Counterparty: "Low", NormalizedCounterparty: "low", Kind: "debit_card", Direction: "debit", Currency: "GBP"},
	}}
	service := batchServiceFake{results: map[string]Result{"Cafe": {Category: "dining", Confidence: .9, Attempted: true}, "Low": {Attempted: true}}, errors: map[string]error{"Broken": errors.New("provider failed: jev-secret-token")}}
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
	if strings.Join(store.attempted, ",") != "ok,bad,noop" {
		t.Fatalf("attempted=%v", store.attempted)
	}
	if strings.Contains(logs.String(), "jev-secret-token") {
		t.Fatalf("logs contain secret: %s", logs.String())
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
}

func TestRunBatchLogsAggregateReasonsWithoutPrivateDetails(t *testing.T) {
	for _, job := range []struct {
		kind, reason string
		suggestion   Suggestion
		attempts     int
	}{
		{"credit_card", "low_confidence", Suggestion{Category: "groceries", Confidence: .5}, 1},
		{"credit_card", "invalid_category", Suggestion{Category: "private-provider-answer", Confidence: .99}, 1},
		{"funds_transfer", "ineligible", Suggestion{}, 0},
	} {
		t.Run(job.reason, func(t *testing.T) {
			var logs bytes.Buffer
			store := &batchStoreFake{candidates: []storage.CategorizationTransaction{
				{ID: "private-transaction-id", Counterparty: "private-counterparty", Kind: job.kind, Direction: "debit"},
			}}
			provider := &jevStub{suggestion: job.suggestion}
			processed, failed, err := RunBatch(context.Background(), store, NewService(nil, provider), "test-model", 1, slog.New(slog.NewJSONHandler(&logs, nil)))
			if err != nil || processed != 1 || failed != 0 || provider.calls != job.attempts || len(store.attempted) != job.attempts || len(store.provenance) != 0 {
				t.Fatalf("unexpected batch outcome: processed=%d failed=%d calls=%d err=%v", processed, failed, provider.calls, err)
			}
			if want := `"` + job.reason + `":1`; !strings.Contains(logs.String(), want) {
				t.Errorf("missing aggregate %s in %q", want, logs.String())
			}
			if strings.Contains(logs.String(), "private-") {
				t.Error("batch log contains private transaction or provider details")
			}
		})
	}
}
