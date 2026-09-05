package storage

import "testing"

func TestTransactionListWhereFiltersAndParameterOrder(t *testing.T) {
	where, args := transactionListWhere(TransactionFilter{Bank: "DBS", Type: "credit_card", Category: "food"})
	want := "1=1 AND t.bank = $1 AND t.kind = $2 AND c.slug = $3"
	if where != want {
		t.Fatalf("where = %q, want %q", where, want)
	}
	if len(args) != 3 || args[0] != "DBS" || args[1] != "credit_card" || args[2] != "food" {
		t.Fatalf("args = %#v", args)
	}
}

func TestTransactionPageSizeIsFixed(t *testing.T) {
	if TransactionPageSize != 25 {
		t.Fatalf("page size = %d", TransactionPageSize)
	}
}
