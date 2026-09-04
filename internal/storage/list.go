package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const TransactionPageSize = 25

type TransactionFilter struct{ Bank, Type, Category string }
type TransactionListItem struct {
	ID                                                                           string
	OccurredAt                                                                   time.Time
	Bank, Type, Kind, MerchantPayee, MaskedSuffix, Category, Currency, Direction string
	AmountMinor                                                                  int64
}
type TransactionPage struct {
	Items       []TransactionListItem
	Total, Page int
	Filter      TransactionFilter
}

type transactionQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func transactionListWhere(filter TransactionFilter) (string, []any) {
	where := []string{"1=1"}
	args := []any{}
	add := func(column, value string) {
		if value != "" {
			args = append(args, value)
			where = append(where, fmt.Sprintf("%s = $%d", column, len(args)))
		}
	}
	add("t.bank", filter.Bank)
	add("t.kind", filter.Type)
	add("c.slug", filter.Category)
	return strings.Join(where, " AND "), args
}

// ListTransactions returns one stable, offset-paginated page. Type filters kind.
func ListTransactions(ctx context.Context, q transactionQuerier, filter TransactionFilter, page int) (TransactionPage, error) {
	if page < 1 {
		page = 1
	}
	whereSQL, args := transactionListWhere(filter)
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM transactions t LEFT JOIN categories c ON c.id=t.category_id WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return TransactionPage{}, err
	}
	offset := (page - 1) * TransactionPageSize
	rows, err := q.Query(ctx, `SELECT t.id, t.occurred_at, t.bank, t.source_type, t.kind,
		COALESCE(NULLIF(t.merchant, ''), NULLIF(t.payee, ''), '—'),
		COALESCE(t.card_suffix, t.from_account_suffix, ''), COALESCE(c.name, 'Uncategorised'),
		t.currency, t.direction, t.amount_minor
		FROM transactions t LEFT JOIN categories c ON c.id=t.category_id
		WHERE `+whereSQL+` ORDER BY t.occurred_at DESC, t.id DESC LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2), append(args, TransactionPageSize, offset)...)
	if err != nil {
		return TransactionPage{}, err
	}
	defer rows.Close()
	out := TransactionPage{Total: total, Page: page, Filter: filter}
	for rows.Next() {
		var item TransactionListItem
		if err := rows.Scan(&item.ID, &item.OccurredAt, &item.Bank, &item.Type, &item.Kind, &item.MerchantPayee, &item.MaskedSuffix, &item.Category, &item.Currency, &item.Direction, &item.AmountMinor); err != nil {
			return TransactionPage{}, err
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return TransactionPage{}, err
	}
	return out, nil
}
