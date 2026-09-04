package postgres

import (
	"context"

	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/transactions"
)

// TransactionListRepository adapts the PostgreSQL storage query to the
// transactions feature's repository interface.
type TransactionListRepository struct {
	Store storage.TransactionStore
}

func (r TransactionListRepository) List(ctx context.Context, filter transactions.Filter, page int) (transactions.Page, error) {
	stored, err := r.Store.ListTransactions(ctx, storage.TransactionFilter{
		Bank:     filter.Bank,
		Type:     filter.Type,
		Category: filter.Category,
	}, page)
	if err != nil {
		return transactions.Page{}, err
	}

	items := make([]transactions.ListItem, 0, len(stored.Items))
	for _, item := range stored.Items {
		items = append(items, transactions.ListItem{
			ID:            item.ID,
			OccurredAt:    item.OccurredAt,
			Bank:          item.Bank,
			Type:          item.Type,
			MerchantPayee: item.MerchantPayee,
			MaskedSuffix:  item.MaskedSuffix,
			Category:      item.Category,
			Currency:      item.Currency,
			Direction:     item.Direction,
			AmountMinor:   item.AmountMinor,
		})
	}
	return transactions.Page{
		Items:  items,
		Total:  stored.Total,
		Page:   stored.Page,
		Filter: filter,
	}, nil
}
