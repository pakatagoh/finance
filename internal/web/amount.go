package web

import "fmt"

// formatAmount renders amount_minor values as signed currency amounts. The
// ingestion contract stores monetary values in the smallest currency unit
// (cents for SGD), so 1250 is displayed as 12.50.
func formatAmount(minor int64, currency, direction string) string {
	sign := "-"
	if direction == "credit" {
		sign = "+"
	}
	if minor < 0 {
		minor = -minor
	}
	return fmt.Sprintf("%s%s %d.%02d", sign, currency, minor/100, minor%100)
}
