package phone

import "testing"

func TestNormalize(t *testing.T) {
	if got := Normalize("+62 812-3456-7890"); got != "6281234567890" {
		t.Fatalf("Normalize() = %q", got)
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid("6281234567890") {
		t.Fatal("expected valid phone")
	}
	if IsValid("123") {
		t.Fatal("expected invalid phone")
	}
}
