package transactions

import (
	"context"
	"time"
)

const PageSize = 25

type Filter struct {
	Bank     string
	Type     string
	Category string
}

type ListItem struct {
	ID            string
	OccurredAt    time.Time
	Bank          string
	Type          string
	Kind          string
	MerchantPayee string
	MaskedSuffix  string
	Category      string
	Currency      string
	Direction     string
	AmountMinor   int64
}

type Page struct {
	Items  []ListItem
	Total  int
	Page   int
	Filter Filter
}

type ListRepository interface {
	List(context.Context, Filter, int) (Page, error)
}

type ListUseCase struct {
	repository ListRepository
}

func NewListUseCase(repository ListRepository) ListUseCase {
	return ListUseCase{repository: repository}
}

func (u ListUseCase) Execute(ctx context.Context, filter Filter, page int) (Page, error) {
	if page < 1 {
		page = 1
	}
	return u.repository.List(ctx, filter, page)
}
