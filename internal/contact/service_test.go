package contact

import "testing"

func TestNormalizeCountry(t *testing.T) {
	t.Run("accepts ISO code", func(t *testing.T) {
		got, err := normalizeCountry("id")
		if err != nil {
			t.Fatalf("normalizeCountry() error = %v", err)
		}
		if got == nil || *got != "ID" {
			t.Fatalf("normalizeCountry() = %v, want ID", got)
		}
	})

	t.Run("rejects long values", func(t *testing.T) {
		_, err := normalizeCountry("INDONESIA")
		if err == nil {
			t.Fatal("normalizeCountry() expected validation error")
		}
	})
}
