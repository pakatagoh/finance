// Package categorization provides deterministic transaction categorization as an
// enrichment step after immutable transaction ingestion.
package categorization

import (
	"context"
	"strings"
	"unicode"
)

const DefaultConfidenceThreshold = 0.85

// CategoryChoices is the closed set Jev may return. Values are category slugs.
var CategoryChoices = []string{
	"income", "housing", "utilities", "groceries", "dining", "transportation",
	"healthcare", "shopping", "entertainment", "subscriptions", "travel",
	"education", "personal", "fees", "other",
}

type Transaction struct {
	Kind, Direction, Currency, Counterparty string
	NormalizedCounterparty, Merchant, Payee string
	AmountMinor                             int64
}

type MappingKey struct {
	Counterparty, Kind, Direction, Currency string
}

type Mapping struct{ Category string }

type Suggestion struct {
	Category   string
	Confidence float64
}

type Provenance string

const (
	ProvenanceNone    Provenance = ""
	ProvenanceMapping Provenance = "mapping"
	ProvenanceJev     Provenance = "jev"
)

type Result struct {
	Category   string
	Confidence float64
	Provenance Provenance
}

// MappingStore supplies human-authored mappings. A nil mapping means no match.
type MappingStore interface {
	Lookup(context.Context, MappingKey) (*Mapping, error)
}

// Jev categorizes using only the supplied closed category set.
type Jev interface {
	Categorize(context.Context, Transaction, []string) (Suggestion, error)
}

type Service struct {
	mappings  MappingStore
	jev       Jev
	threshold float64
}

func NewService(mappings MappingStore, jev Jev) Service {
	return NewServiceWithThreshold(mappings, jev, DefaultConfidenceThreshold)
}

func NewServiceWithThreshold(mappings MappingStore, jev Jev, threshold float64) Service {
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}
	return Service{mappings: mappings, jev: jev, threshold: threshold}
}

// Categorize returns only a high-confidence, canonical category. Ineligible or
// unresolved transactions return the zero Result, allowing safe retries.
func (s Service) Categorize(ctx context.Context, tx Transaction) (Result, error) {
	key, ok := KeyFor(tx)
	if !ok {
		return Result{}, nil
	}
	if s.mappings != nil {
		mapping, err := s.mappings.Lookup(ctx, key)
		if err != nil {
			return Result{}, err
		}
		if mapping != nil {
			if category, valid := canonicalCategory(mapping.Category); valid {
				return Result{Category: category, Confidence: 1, Provenance: ProvenanceMapping}, nil
			}
		}
	}
	if s.jev == nil {
		return Result{}, nil
	}
	suggestion, err := s.jev.Categorize(ctx, tx, append([]string(nil), CategoryChoices...))
	if err != nil || suggestion.Confidence < s.threshold {
		return Result{}, nil
	}
	category, valid := canonicalCategory(suggestion.Category)
	if !valid {
		return Result{}, nil
	}
	return Result{Category: category, Confidence: suggestion.Confidence, Provenance: ProvenanceJev}, nil
}

func KeyFor(tx Transaction) (MappingKey, bool) {
	kind := strings.ToLower(strings.TrimSpace(tx.Kind))
	direction := strings.ToLower(strings.TrimSpace(tx.Direction))
	if kind != "credit_card" && kind != "debit_card" && kind != "paynow" {
		return MappingKey{}, false
	}
	if kind == "paynow" && direction != "debit" {
		return MappingKey{}, false
	}
	counterparty := NormalizeCounterparty(tx.Counterparty)
	if counterparty == "" {
		return MappingKey{}, false
	}
	return MappingKey{Counterparty: counterparty, Kind: kind, Direction: direction, Currency: strings.ToLower(strings.TrimSpace(tx.Currency))}, true
}

func NormalizeCounterparty(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(' ')
	}
	value = strings.Join(strings.Fields(b.String()), " ")
	switch value {
	case "", "n a", "na", "none", "null", "nil", "unknown", "unk", "not available", "not provided", "-":
		return ""
	default:
		return value
	}
}

func canonicalCategory(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, choice := range CategoryChoices {
		if value == choice {
			return value, true
		}
	}
	return "", false
}
