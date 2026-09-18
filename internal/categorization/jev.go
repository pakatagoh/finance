package categorization

import (
	"context"
	"fmt"

	"github.com/pakatagoh/finance/internal/jev"
)

// JevClient is the typed Jev boundary used by the categorizer.
type JevClient interface {
	Choice(context.Context, jev.ChoiceRequest) (jev.ChoiceResponse, error)
}

type JevCategorizer struct {
	client JevClient
	model  string
}

func NewJevCategorizer(client JevClient, model string) JevCategorizer {
	return JevCategorizer{client: client, model: model}
}

func (j JevCategorizer) Categorize(ctx context.Context, tx Transaction, allowed []string) (Suggestion, error) {
	criteria := make(map[string]string, len(allowed))
	for _, category := range allowed {
		criteria[category] = category
	}
	request := jev.ChoiceRequest{
		Model: j.model,
		State: map[string]any{
			"counterparty":            tx.Counterparty,
			"normalized_counterparty": tx.NormalizedCounterparty,
			"kind":                    tx.Kind, "direction": tx.Direction,
			"amount_minor": tx.AmountMinor, "currency": tx.Currency,
			"merchant": tx.Merchant, "payee": tx.Payee,
		},
		Questions: map[string]jev.ChoiceQuestion{
			"category": {Type: "choice", Instructions: "Choose the best canonical transaction category.", Criteria: criteria},
		},
	}
	response, err := j.client.Choice(ctx, request)
	if err != nil {
		return Suggestion{}, err
	}
	answer, ok := response.Answers["category"]
	if !ok {
		return Suggestion{}, fmt.Errorf("Jev response omitted category answer")
	}
	return Suggestion{Category: answer.Choice, Confidence: answer.Confidence}, nil
}
