package contact

import "testing"

func TestParseInstagramScopedID(t *testing.T) {
	id, ok := ParseInstagramScopedID("ig:1028733076273916")
	if !ok || id != "1028733076273916" {
		t.Fatalf("got %q %v", id, ok)
	}

	if _, ok := ParseInstagramScopedID("+6282288812003"); ok {
		t.Fatal("whatsapp phone must not parse as igsid")
	}
	if _, ok := ParseInstagramScopedID("ig:+6282288812003"); ok {
		t.Fatal("non-numeric ig key must be rejected")
	}
	if _, ok := ParseInstagramScopedID("ig:"); ok {
		t.Fatal("empty ig key must be rejected")
	}
}

func TestIsValidInstagramScopedID(t *testing.T) {
	if !IsValidInstagramScopedID("1028733076273916") {
		t.Fatal("expected valid")
	}
	if IsValidInstagramScopedID("+6282288812003") {
		t.Fatal("phone must be invalid")
	}
	if IsValidInstagramScopedID("") {
		t.Fatal("empty must be invalid")
	}
}
