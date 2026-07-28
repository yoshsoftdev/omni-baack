package automation

import (
	"testing"
)

func TestSlugifyAndValidate(t *testing.T) {
	if got := Slugify("Send Invoice"); got != "send-invoice" {
		t.Fatalf("Slugify = %q", got)
	}
	if !IsValidSlug("send-invoice") {
		t.Fatal("expected valid slug")
	}
	if IsValidSlug("Send Invoice") {
		t.Fatal("expected invalid slug")
	}
}

func TestHashAPIKeyStable(t *testing.T) {
	a := HashAPIKey("oc_auto_abc")
	b := HashAPIKey("oc_auto_abc")
	if a != b || a == "" {
		t.Fatalf("hash mismatch %q %q", a, b)
	}
}

func TestGenerateAPIKeySecret(t *testing.T) {
	secret, prefix, hash, err := GenerateAPIKeySecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) < 20 || prefix == "" || hash == "" {
		t.Fatalf("unexpected secret parts secret=%q prefix=%q hash=%q", secret, prefix, hash)
	}
	if HashAPIKey(secret) != hash {
		t.Fatal("hash does not match secret")
	}
}

func TestNormalizeScopes(t *testing.T) {
	got := normalizeScopes(nil)
	if len(got) != 1 || got[0] != "send_template" {
		t.Fatalf("got %#v", got)
	}
}

func TestNewIDs(t *testing.T) {
	if got := NewEndpointID(); len(got) < 5 || got[:4] != "ept_" {
		t.Fatalf("endpoint id %q", got)
	}
	if got := NewAPIKeyID(); got[:4] != "key_" {
		t.Fatalf("key id %q", got)
	}
	if got := NewExecutionID(); got[:5] != "exec_" {
		t.Fatalf("exec id %q", got)
	}
}
