package phone

import "testing"

func TestValidateIndonesiaE164(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"+628123456789", true},
		{"+62 812-3456-789", true},
		{"628123456789", false},
		{"+60123456789", false},
		{"+628123", false},
		{"+08123456789", false},
	}

	for _, tt := range tests {
		_, err := ValidateIndonesiaE164(tt.input)
		if tt.valid && err != nil {
			t.Fatalf("ValidateIndonesiaE164(%q) unexpected error: %v", tt.input, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("ValidateIndonesiaE164(%q) expected error", tt.input)
		}
	}
}

func TestToMetaRecipient(t *testing.T) {
	if got := ToMetaRecipient("+628123456789"); got != "628123456789" {
		t.Fatalf("ToMetaRecipient() = %q", got)
	}
}

func TestValidateNumericCode(t *testing.T) {
	if err := ValidateNumericCode("464646"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateNumericCode("abc"); err == nil {
		t.Fatal("expected error for non-numeric code")
	}
}
