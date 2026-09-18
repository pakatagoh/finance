package storage

import (
	"testing"
	"time"
)

func TestNormalizeCounterparty(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"  ACME  ", "acme"},
		{"Acme   Coffee", "acme coffee"},
		{"ACME, LTD.", "acme ltd"},
		{"", ""},
	} {
		if got := NormalizeCounterparty(tt.input); got != tt.want {
			t.Errorf("NormalizeCounterparty(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMappingSourcePriority(t *testing.T) {
	if !CanReplaceMapping(MappingSourceJev, MappingSourceUser) {
		t.Fatal("user mapping should replace Jev mapping")
	}
	if CanReplaceMapping(MappingSourceUser, MappingSourceJev) {
		t.Fatal("Jev mapping must not replace user mapping")
	}
	if !CanReplaceMapping(MappingSourceJev, MappingSourceJev) {
		t.Fatal("same-source mapping should be refreshable")
	}
}

func TestCategoryProvenanceEqualIsIdempotent(t *testing.T) {
	at := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	confidence := 0.9
	model := "model"
	a := CategoryProvenance{CategoryID: "cat", Source: MappingSourceJev, Confidence: &confidence, Model: &model, CategorizedAt: at}
	if !a.Equal(a) {
		t.Fatal("equal provenance should be idempotent")
	}
}
