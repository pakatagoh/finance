package transactions

import (
	"context"
	"testing"
)

type repositoryStub struct {
	gotFilter Filter
	gotPage   int
	page      Page
}

func (s *repositoryStub) List(_ context.Context, filter Filter, page int) (Page, error) {
	s.gotFilter = filter
	s.gotPage = page
	return s.page, nil
}

func TestListUseCaseNormalizesPageAndDelegates(t *testing.T) {
	repo := &repositoryStub{}
	uc := NewListUseCase(repo)

	wantFilter := Filter{Bank: "DBS", Type: "card", Category: "food"}
	_, err := uc.Execute(context.Background(), wantFilter, 0)
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotFilter != wantFilter || repo.gotPage != 1 {
		t.Fatalf("repository args = %+v page %d", repo.gotFilter, repo.gotPage)
	}
}
