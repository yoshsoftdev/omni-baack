package contact

import "testing"

func TestResolveInboxDisplayName(t *testing.T) {
	c := &Contact{
		ContactName:  "Inbox Label",
		CustomerName: "YoShandy",
		Phone:        "+6282288812003",
	}
	if got := ResolveInboxDisplayName(c); got != "Inbox Label" {
		t.Fatalf("ResolveInboxDisplayName() = %q, want Inbox Label", got)
	}

	c.ContactName = ""
	if got := ResolveInboxDisplayName(c); got != "YoShandy" {
		t.Fatalf("fallback customer_name = %q", got)
	}

	c.CustomerName = ""
	if got := ResolveInboxDisplayName(c); got != "+6282288812003" {
		t.Fatalf("fallback phone = %q", got)
	}
}

func TestNewInboundNames(t *testing.T) {
	contactName, customerName := NewInboundNames("YoShandy", "+6282288812003")
	if customerName != "YoShandy" {
		t.Fatalf("customer_name = %q", customerName)
	}
	if contactName != "YoShandy" {
		t.Fatalf("contact_name = %q", contactName)
	}

	contactName, customerName = NewInboundNames("", "+6282288812003")
	if customerName != "" {
		t.Fatalf("empty profile customer_name = %q", customerName)
	}
	if contactName != "+6282288812003" {
		t.Fatalf("phone fallback contact_name = %q", contactName)
	}
}

func TestShouldAutoUpdateContactName(t *testing.T) {
	c := &Contact{
		ContactName:  "Custom Agent Label",
		CustomerName: "YoShandy",
		Phone:        "+6282288812003",
	}
	if ShouldAutoUpdateContactName(c) {
		t.Fatal("expected manual contact_name to be preserved")
	}

	c.ContactName = c.CustomerName
	if !ShouldAutoUpdateContactName(c) {
		t.Fatal("expected synced contact_name to be auto-updated")
	}

	c.ContactName = "+6282288812003"
	if !ShouldAutoUpdateContactName(c) {
		t.Fatal("expected phone-only contact_name to be auto-updated")
	}

	c.ContactName = "Instagram User"
	c.CustomerName = "Instagram User"
	if !ShouldAutoUpdateContactName(c) {
		t.Fatal("expected Instagram User placeholder to be auto-updated")
	}
}
