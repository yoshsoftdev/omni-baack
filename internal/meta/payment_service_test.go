package meta

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
)

func TestDefaultPaymentSetupURL(t *testing.T) {
	businessID := "biz-123"
	wabaID := "waba-456"
	ch := &channel.Channel{
		BusinessID: &businessID,
		WABAID:     &wabaID,
	}
	got := defaultPaymentSetupURL(ch)
	if got == "" {
		t.Fatal("expected payment setup URL")
	}
	if want := "business_id=biz-123"; !contains(got, want) {
		t.Fatalf("url = %q, want substring %q", got, want)
	}
}

func TestApplyPaymentURLPlaceholders(t *testing.T) {
	businessID := "biz-1"
	wabaID := "waba-2"
	phoneID := "phone-3"
	ch := &channel.Channel{
		BusinessID:    &businessID,
		WABAID:        &wabaID,
		PhoneNumberID: &phoneID,
	}
	got := applyPaymentURLPlaceholders("https://example.test/{business_id}/{waba_id}/{phone_number_id}", ch)
	want := "https://example.test/biz-1/waba-2/phone-3"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func contains(s, part string) bool {
	return len(part) == 0 || (len(s) >= len(part) && indexOf(s, part) >= 0)
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

func TestPaymentStatusConstants(t *testing.T) {
	if PaymentStatusConfigured == PaymentStatusMissing {
		t.Fatal("payment status constants must differ")
	}
	_ = uuid.Nil
}
