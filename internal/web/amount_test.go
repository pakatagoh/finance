package web

import "testing"

func TestFormatAmount(t *testing.T) {
	for _, tc := range []struct {
		minor     int64
		currency  string
		direction string
		want      string
	}{
		{1250, "SGD", "debit", "-SGD 12.50"},
		{100, "SGD", "credit", "+SGD 1.00"},
		{5, "SGD", "debit", "-SGD 0.05"},
	} {
		if got := formatAmount(tc.minor, tc.currency, tc.direction); got != tc.want {
			t.Errorf("formatAmount(%d, %q, %q) = %q, want %q", tc.minor, tc.currency, tc.direction, got, tc.want)
		}
	}
}
