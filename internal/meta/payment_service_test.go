package meta

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
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

func TestClassifyPaymentStatus(t *testing.T) {
	t.Run("funding id configured", func(t *testing.T) {
		got := classifyPaymentStatus("funding-1", "ACTIVE", "verified", "waba-1", "biz-1", "waba-1", "biz-1", "", false, nil)
		if got.Status != PaymentStatusConfigured || got.CheckSource != PaymentCheckSourceFunding {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("health payment error 141006", func(t *testing.T) {
		health := &metapkg.WABAHealthStatus{
			CanSendMessage: "BLOCKED",
			Entities: []metapkg.WABAHealthEntity{{
				EntityType:     "WABA",
				CanSendMessage: "BLOCKED",
				Errors: []metapkg.WABAHealthError{{
					ErrorCode:        metapkg.HealthErrorPaymentMethod,
					ErrorDescription: "There is an error with the payment method.",
				}},
			}},
		}
		got := classifyPaymentStatus("", "ACTIVE", "not_verified", "waba-1", "biz-1", "waba-1", "biz-1", "(#10)", true, health)
		if got.Status != PaymentStatusMissing {
			t.Fatalf("status=%q", got.Status)
		}
		if got.CheckSource != PaymentCheckSourceHealth {
			t.Fatalf("source=%q", got.CheckSource)
		}
		if got.HealthPaymentErrorCode != metapkg.HealthErrorPaymentMethod {
			t.Fatalf("code=%d", got.HealthPaymentErrorCode)
		}
	})

	t.Run("health available means payment ok for tech provider", func(t *testing.T) {
		health := &metapkg.WABAHealthStatus{CanSendMessage: "AVAILABLE"}
		got := classifyPaymentStatus("", "ACTIVE", "verified", "waba-1", "biz-1", "waba-1", "biz-1", "(#10)", true, health)
		if got.Status != PaymentStatusConfigured {
			t.Fatalf("got %+v", got)
		}
		if got.CheckSource != PaymentCheckSourceHealth {
			t.Fatalf("source=%q", got.CheckSource)
		}
	})

	t.Run("restricted overrides", func(t *testing.T) {
		got := classifyPaymentStatus("funding-1", "RESTRICTED", "verified", "waba-1", "biz-1", "waba-1", "biz-1", "", false, nil)
		if got.Status != PaymentStatusRestricted {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestIsBusinessVerified(t *testing.T) {
	if !IsBusinessVerified("verified") {
		t.Fatal("expected verified")
	}
}
