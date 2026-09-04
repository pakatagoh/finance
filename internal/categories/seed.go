// Package categories owns category persistence and category-related commands.
package categories

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const initialCategoriesSeed = "initial-categories-v1"

// InitialCategory is a category included in the initial seed.
type InitialCategory struct {
	Slug string
	Name string
}

// InitialCategories is the deliberately versioned initial category set.
var InitialCategories = []InitialCategory{
	{Slug: "income", Name: "Income"},
	{Slug: "housing", Name: "Housing"},
	{Slug: "utilities", Name: "Utilities"},
	{Slug: "groceries", Name: "Groceries"},
	{Slug: "dining", Name: "Dining"},
	{Slug: "transportation", Name: "Transportation"},
	{Slug: "healthcare", Name: "Healthcare"},
	{Slug: "shopping", Name: "Shopping"},
	{Slug: "entertainment", Name: "Entertainment"},
	{Slug: "subscriptions", Name: "Subscriptions"},
	{Slug: "travel", Name: "Travel"},
	{Slug: "education", Name: "Education"},
	{Slug: "personal", Name: "Personal"},
	{Slug: "fees", Name: "Fees"},
}

// Beginner starts a database transaction.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// SeedInitialCategories inserts the initial categories exactly once. The seed
// marker and all category rows are committed atomically in the same transaction.
func SeedInitialCategories(ctx context.Context, db Beginner) (completed bool, err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin category seed transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var marker string
	err = tx.QueryRow(ctx, `
		INSERT INTO seed_runs (name) VALUES ($1)
		ON CONFLICT (name) DO NOTHING
		RETURNING name`, initialCategoriesSeed).Scan(&marker)
	if err == pgx.ErrNoRows {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return false, fmt.Errorf("already-completed seed rollback: %w", rollbackErr)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("record category seed: %w", err)
	}

	values := make([]string, 0, len(InitialCategories))
	args := make([]any, 0, len(InitialCategories)*2)
	for i, category := range InitialCategories {
		base := i*2 + 1
		values = append(values, fmt.Sprintf("($%d, $%d)", base, base+1))
		args = append(args, category.Slug, category.Name)
	}
	query := `INSERT INTO categories (slug, name) VALUES ` + strings.Join(values, ", ")
	if _, err = tx.Exec(ctx, query, args...); err != nil {
		return false, fmt.Errorf("insert initial categories: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit category seed: %w", err)
	}
	return true, nil
}
