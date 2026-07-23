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
	if PaymentStatusRestricted == PaymentStatusConfigured {
		t.Fatal("restricted must differ from configured")
	}
	_ = uuid.Nil
}

func TestClassifyPaymentStatus(t *testing.T) {
	t.Run("missing funding", func(t *testing.T) {
		got := classifyPaymentStatus("", "ACTIVE")
		if got.Status != PaymentStatusMissing || got.Message != PaymentMessageMissing {
			t.Fatalf("got %+v", got)
		}
		if got.HasPrimaryFunding {
			t.Fatal("expected no funding")
		}
	})

	t.Run("payment ok", func(t *testing.T) {
		got := classifyPaymentStatus("funding-1", "ACTIVE")
		if got.Status != PaymentStatusConfigured || got.Message != PaymentMessageOK {
			t.Fatalf("got %+v", got)
		}
		if !got.HasPrimaryFunding || got.PrimaryFundingID != "funding-1" {
			t.Fatalf("funding fields = %+v", got)
		}
	})

	t.Run("restricted overrides funding", func(t *testing.T) {
		got := classifyPaymentStatus("funding-1", "RESTRICTED")
		if got.Status != PaymentStatusRestricted || got.Message != PaymentMessageRestricted {
			t.Fatalf("got %+v", got)
		}
		if got.WABAStatus != "RESTRICTED" {
			t.Fatalf("waba status = %q", got.WABAStatus)
		}
	})

	t.Run("restricted case insensitive", func(t *testing.T) {
		got := classifyPaymentStatus("", "restricted")
		if got.Status != PaymentStatusRestricted {
			t.Fatalf("got %+v", got)
		}
	})
}
