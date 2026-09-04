package transactions

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pakatagoh/finance/internal/storage"
)

type DetailRepository interface {
	storage.TransactionReader
	storage.TransactionEditor
	ActiveCategories(context.Context) ([]storage.Category, error)
}

type DetailUseCase struct {
	repository DetailRepository
}

func NewDetailUseCase(repository DetailRepository) DetailUseCase {
	return DetailUseCase{repository: repository}
}

func (u DetailUseCase) Load(ctx context.Context, id string) (storage.Transaction, []storage.Category, error) {
	tx, err := u.repository.GetTransaction(ctx, id)
	if err != nil {
		return storage.Transaction{}, nil, err
	}
	categories, err := u.repository.ActiveCategories(ctx)
	if err != nil {
		return tx, nil, err
	}
	return tx, categories, nil
}

func (u DetailUseCase) Save(ctx context.Context, id, categoryID, rawNotes string) (storage.Transaction, error) {
	tx, err := u.repository.GetTransaction(ctx, id)
	if err != nil {
		return storage.Transaction{}, err
	}

	category := strings.TrimSpace(categoryID)
	if category == "" {
		tx.CategoryID = nil
	} else {
		tx.CategoryID = &category
	}
	notes, err := normalizeNotes(rawNotes)
	if err != nil {
		return tx, err
	}
	if notes == "" {
		tx.Notes = nil
	} else {
		tx.Notes = &notes
	}

	saved, err := u.repository.UpdateEnrichment(ctx, id, tx.CategoryID, tx.Notes)
	if err != nil {
		return tx, err
	}
	return saved, nil
}

func normalizeNotes(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if utf8.RuneCountInString(value) > 2000 {
		return "", fmt.Errorf("notes must be 2,000 characters or fewer")
	}
	return value, nil
}
