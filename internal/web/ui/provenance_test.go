package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/transactions"
)

func renderDocument(t *testing.T, component interface {
	Render(context.Context, interface{ Write([]byte) (int, error) }) error
}) *goquery.Document {
	t.Helper()
	var body bytes.Buffer
	if err := component.Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body.String()))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func provenanceTransaction(source string, confidence *float64, model *string) storage.Transaction {
	return storage.Transaction{ID: "tx-1", OccurredAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Bank: "DBS", SourceType: "card", Kind: "debit_card", Currency: "SGD", AmountMinor: 1234, Direction: "debit", CategorySource: source, CategoryConfidence: confidence, CategoryModel: model}
}

func TestTransactionDetailOmitsProvenanceWhenAbsent(t *testing.T) {
	doc := renderDetailDocument(t, provenanceTransaction("", nil, nil))
	if got := doc.Find("#category-provenance-heading").Length(); got != 0 {
		t.Fatalf("provenance heading count = %d, want 0", got)
	}
}

func TestTransactionDetailRendersSemanticProvenanceDetails(t *testing.T) {
	confidence := 0.875
	model := "jev-v2"
	doc := renderDetailDocument(t, provenanceTransaction("jev", &confidence, &model))
	section := doc.Find("section[aria-labelledby='category-provenance-heading']")
	if section.Length() != 1 {
		t.Fatalf("provenance section count = %d, want 1", section.Length())
	}
	if got := section.Find("dt").Map(func(_ int, s *goquery.Selection) string { return strings.TrimSpace(s.Text()) }); strings.Join(got, "|") != "Source|Model|Confidence" {
		t.Fatalf("provenance labels = %v", got)
	}
	if !strings.Contains(section.Text(), "jev-v2") || !strings.Contains(section.Text(), "88%") {
		t.Fatalf("provenance details missing: %q", section.Text())
	}
}

func TestTransactionsListShowsAccessibleJevIndicatorOnlyForJev(t *testing.T) {
	page := transactions.Page{Items: []transactions.ListItem{{ID: "jev", Category: "Food", CategorySource: "jev"}, {ID: "user", Category: "Bills", CategorySource: "user"}, {ID: "none", Category: "Uncategorised"}}}
	doc := renderTransactionsDocument(t, page)
	indicator := doc.Find("[role='img'][aria-label='AI categorized by Jev'][title='Categorized by Jev']")
	if indicator.Length() != 1 {
		t.Fatalf("AI indicator count = %d, want 1", indicator.Length())
	}
	if got := indicator.Text(); got != "✨" {
		t.Fatalf("indicator text = %q", got)
	}
}

func renderDetailDocument(t *testing.T, tx storage.Transaction) *goquery.Document {
	t.Helper()
	var body bytes.Buffer
	if err := TransactionDetail("nonce", tx, nil, "/transactions", "", "", "").Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body.String()))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func renderTransactionsDocument(t *testing.T, page transactions.Page) *goquery.Document {
	t.Helper()
	var body bytes.Buffer
	if err := TransactionsResults("nonce", page).Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body.String()))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
