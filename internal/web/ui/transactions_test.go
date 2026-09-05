package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pakatagoh/finance/internal/transactions"
)

func TestTransactionsResultsEmptyState(t *testing.T) {
	var body bytes.Buffer
	page := transactions.Page{Filter: transactions.Filter{Bank: "DBS", Type: "card"}, Page: 3}
	if err := TransactionsResults(page).Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	got := body.String()
	for _, want := range []string{
		"No transactions yet",
		"New transactions will appear here after they are received from the bank tracker.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty state missing %q in %s", want, got)
		}
	}
	for _, unwanted := range []string{"No transactions found.", "Import", "Get started", "<tr>"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("empty state contains unwanted %q in %s", unwanted, got)
		}
	}
	if !strings.Contains(got, `name="bank" value="DBS"`) || !strings.Contains(got, `name="type" value="card"`) {
		t.Errorf("empty state did not preserve filters: %s", got)
	}
}

func TestTransactionDisplayHelpers(t *testing.T) {
	got := singaporeTime(time.Date(2026, 1, 2, 16, 0, 0, 0, time.UTC))
	if got != "03 Jan 2026 00:00" {
		t.Fatalf("Singapore time = %q", got)
	}
	if signedAmount(1234, "SGD", "debit") != "-SGD 12.34" || signedAmount(1234, "SGD", "credit") != "+SGD 12.34" {
		t.Fatal("signed amount formatting")
	}
	if suffix("1234") != "•••• 1234" {
		t.Fatal("suffix masking")
	}
	if pageURL(transactions.Filter{Bank: "DBS", Type: "card purchase"}, 2) != "/transactions?bank=DBS&page=2&type=card+purchase" {
		t.Fatal("page URL encoding")
	}
}

func TestTransactionKindAndDirectionLabels(t *testing.T) {
	if transactionKind("card_purchase") != "Credit card" || transactionKind("funds_transfer") != "Fund transfer" || transactionKind("paynow") != "PayNow" {
		t.Fatal("transaction kind formatting")
	}
}
