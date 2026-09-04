package categories

import "testing"

func TestInitialCategoriesAreThePlannedSet(t *testing.T) {
	if got, want := len(InitialCategories), 14; got != want {
		t.Fatalf("initial category count = %d, want %d", got, want)
	}
	seenSlugs := make(map[string]bool, len(InitialCategories))
	seenNames := make(map[string]bool, len(InitialCategories))
	for _, category := range InitialCategories {
		if category.Slug == "" || category.Name == "" {
			t.Fatalf("initial category has empty field: %#v", category)
		}
		if seenSlugs[category.Slug] || seenNames[category.Name] {
			t.Fatalf("duplicate initial category: %#v", category)
		}
		seenSlugs[category.Slug] = true
		seenNames[category.Name] = true
	}
}

func TestInitialCategoriesSeedNameIsVersioned(t *testing.T) {
	if initialCategoriesSeed != "initial-categories-v1" {
		t.Fatalf("seed name = %q, want initial-categories-v1", initialCategoriesSeed)
	}
}
