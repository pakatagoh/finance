package categorization

import (
	"context"
	"errors"
	"testing"
)

type mappingStoreStub struct {
	mapping *Mapping
	key     MappingKey
	calls   int
}

func (s *mappingStoreStub) Lookup(_ context.Context, key MappingKey) (*Mapping, error) {
	s.calls++
	s.key = key
	return s.mapping, nil
}

type jevStub struct {
	suggestion Suggestion
	err        error
	calls      int
	allowed    []string
}

func (s *jevStub) Categorize(_ context.Context, _ Transaction, allowed []string) (Suggestion, error) {
	s.calls++
	s.allowed = allowed
	return s.suggestion, s.err
}

func TestServiceUsesHumanMappingForEligibleDebit(t *testing.T) {
	store := &mappingStoreStub{mapping: &Mapping{Category: "groceries"}}
	jev := &jevStub{}
	svc := NewService(store, jev)
	out, err := svc.Categorize(context.Background(), Transaction{Kind: "debit_card", Direction: "DEBIT", Currency: " gbp ", Counterparty: "  ACME   Market "})
	if err != nil {
		t.Fatal(err)
	}
	if out.Category != "groceries" || out.Provenance != ProvenanceMapping {
		t.Fatalf("got %#v", out)
	}
	if jev.calls != 0 {
		t.Fatal("Jev called despite mapping")
	}
	if store.key != (MappingKey{Counterparty: "acme market", Kind: "debit_card", Direction: "debit", Currency: "gbp"}) {
		t.Fatalf("key %#v", store.key)
	}
}

func TestServiceFallsBackToHighConfidenceJevAndCanonicalChoices(t *testing.T) {
	store := &mappingStoreStub{}
	jev := &jevStub{suggestion: Suggestion{Category: "Dining", Confidence: .9}}
	out, err := NewService(store, jev).Categorize(context.Background(), Transaction{Kind: "credit_card", Direction: "credit", Currency: "USD", Counterparty: "Cafe"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Category != "dining" || out.Provenance != ProvenanceJev || out.Confidence != .9 {
		t.Fatalf("got %#v", out)
	}
	if len(jev.allowed) == 0 || jev.allowed[len(jev.allowed)-1] != "other" {
		t.Fatalf("allowed %#v", jev.allowed)
	}
}

func TestServiceDoesNotCategorizeIneligibleOrMeaningless(t *testing.T) {
	for _, tx := range []Transaction{
		{Kind: "funds_transfer", Direction: "debit", Counterparty: "Useful"},
		{Kind: "incoming_transfer", Direction: "credit", Counterparty: "Useful"},
		{Kind: "reversal", Direction: "credit", Counterparty: "Useful"},
		{Kind: "paynow", Direction: "credit", Counterparty: "Useful"},
		{Kind: "debit_card", Direction: "debit", Counterparty: "—"},
	} {
		store, jev := &mappingStoreStub{}, &jevStub{}
		out, err := NewService(store, jev).Categorize(context.Background(), tx)
		if err != nil {
			t.Fatal(err)
		}
		if out.Category != "" || out.Provenance != ProvenanceNone || store.calls != 0 || jev.calls != 0 {
			t.Fatalf("tx %#v => %#v", tx, out)
		}
	}
}

func TestServiceLeavesUnchangedOnJevFailureOrLowConfidence(t *testing.T) {
	out, err := NewService(&mappingStoreStub{}, &jevStub{err: errors.New("unavailable")}).Categorize(context.Background(), Transaction{Kind: "paynow", Direction: "debit", Counterparty: "Shop"})
	if err == nil || out != (Result{Attempted: true}) {
		t.Fatalf("failure result=%#v err=%v", out, err)
	}
	out, err = NewService(&mappingStoreStub{}, &jevStub{suggestion: Suggestion{Category: "groceries", Confidence: .84}}).Categorize(context.Background(), Transaction{Kind: "paynow", Direction: "debit", Counterparty: "Shop"})
	if err != nil || out != (Result{Attempted: true}) {
		t.Fatalf("low-confidence result=%#v err=%v", out, err)
	}
}

func TestServiceThresholdCanBeConfiguredAndRejectsNonCanonicalJev(t *testing.T) {
	jev := &jevStub{suggestion: Suggestion{Category: "made-up", Confidence: .99}}
	out, err := NewServiceWithThreshold(&mappingStoreStub{}, jev, .95).Categorize(context.Background(), Transaction{Kind: "debit_card", Direction: "debit", Counterparty: "Shop"})
	if err != nil {
		t.Fatal(err)
	}
	if out != (Result{Attempted: true}) {
		t.Fatalf("got %#v", out)
	}
}

func TestMeaningfulCounterpartyNormalization(t *testing.T) {
	if got := NormalizeCounterparty("  ACME,  LTD. "); got != "acme ltd" {
		t.Fatalf("got %q", got)
	}
	if NormalizeCounterparty("N/A") != "" || NormalizeCounterparty("unknown") != "" {
		t.Fatal("meaningless counterparty accepted")
	}
}
