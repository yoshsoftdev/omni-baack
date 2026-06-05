package phone

import (
	"regexp"
	"strings"
)

var nonDigit = regexp.MustCompile(`\D`)

// Normalize strips non-digits and leading zeros for storage.
func Normalize(raw string) string {
	digits := nonDigit.ReplaceAllString(strings.TrimSpace(raw), "")
	digits = strings.TrimLeft(digits, "0")
	return digits
}

// IsValid returns true when the normalized phone has 8-15 digits.
func IsValid(raw string) bool {
	n := Normalize(raw)
	return len(n) >= 8 && len(n) <= 15
}
