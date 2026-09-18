package categorization

import (
	"context"
	"reflect"
	"testing"

	"github.com/pakatagoh/finance/internal/jev"
)

type choiceCapture struct {
	request jev.ChoiceRequest
}

func (c *choiceCapture) Choice(_ context.Context, request jev.ChoiceRequest) (jev.ChoiceResponse, error) {
	c.request = request
	return jev.ChoiceResponse{Answers: map[string]jev.ChoiceAnswer{
		"category": {Type: "choice", Choice: "dining", Confidence: .91},
	}}, nil
}

func TestJevCategorizerSendsCanonicalCriteriaAndTransactionEvidence(t *testing.T) {
	client := &choiceCapture{}
	got, err := NewJevCategorizer(client, "jev-test").Categorize(context.Background(), Transaction{
		Counterparty: "Cafe Ltd", NormalizedCounterparty: "cafe ltd", Kind: "debit_card",
		Direction: "debit", AmountMinor: 1234, Currency: "GBP", Merchant: "Cafe Ltd", Payee: "Alex",
	}, append([]string(nil), CategoryChoices...))
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "dining" || got.Confidence != .91 {
		t.Fatalf("suggestion = %#v", got)
	}
	question, ok := client.request.Questions["category"]
	if !ok || question.Type != "choice" {
		t.Fatalf("question = %#v", client.request.Questions)
	}
	if !reflect.DeepEqual(question.Criteria, func() map[string]string {
		out := make(map[string]string, len(CategoryChoices))
		for _, c := range CategoryChoices {
			out[c] = c
		}
		return out
	}()) {
		t.Fatalf("criteria = %#v", question.Criteria)
	}
	wantState := map[string]any{"counterparty": "Cafe Ltd", "normalized_counterparty": "cafe ltd", "kind": "debit_card", "direction": "debit", "amount_minor": int64(1234), "currency": "GBP", "merchant": "Cafe Ltd", "payee": "Alex"}
	if !reflect.DeepEqual(client.request.State, wantState) {
		t.Fatalf("state = %#v", client.request.State)
	}
}
