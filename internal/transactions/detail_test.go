package transactions_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/transactions"
)

type detailRepositoryStub struct {
	tx              storage.Transaction
	categories      []storage.Category
	updated         bool
	notes           *string
	applyToMatching bool
}

func (s *detailRepositoryStub) GetTransaction(context.Context, string) (storage.Transaction, error) {
	return s.tx, nil
}
func (s *detailRepositoryStub) ActiveCategories(context.Context) ([]storage.Category, error) {
	return s.categories, nil
}
func (s *detailRepositoryStub) UpdateEnrichment(_ context.Context, _ string, _ *string, notes *string, applyToMatching bool) (storage.Transaction, error) {
	s.updated = true
	s.notes = notes
	s.applyToMatching = applyToMatching
	return s.tx, nil
}

func TestDetailUseCasePassesApplyToMatchingChoice(t *testing.T) {
	repo := &detailRepositoryStub{tx: storage.Transaction{ID: "abc"}}
	_, err := transactions.NewDetailUseCase(repo).Save(context.Background(), "abc", "cat", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.applyToMatching {
		t.Fatal("apply-to-matching choice was not passed to the repository")
	}
}

func TestDetailUseCaseNormalizesNotesBeforeSaving(t *testing.T) {
	repo := &detailRepositoryStub{tx: storage.Transaction{ID: "abc"}}
	uc := transactions.NewDetailUseCase(repo)

	_, err := uc.Save(context.Background(), "abc", "", "  hello  ", false)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.updated || repo.notes == nil || *repo.notes != "hello" {
		t.Fatalf("updated=%v notes=%v", repo.updated, repo.notes)
	}
}

func TestDetailUseCaseRejectsOversizedNotesBeforeSaving(t *testing.T) {
	repo := &detailRepositoryStub{tx: storage.Transaction{ID: "abc"}}
	uc := transactions.NewDetailUseCase(repo)

	_, err := uc.Save(context.Background(), "abc", "", strings.Repeat("x", 2001), false)
	if err == nil || repo.updated {
		t.Fatalf("err=%v updated=%v", err, repo.updated)
	}
}
