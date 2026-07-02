package channelsync

import (
	"testing"
)

func TestPickDisplayName(t *testing.T) {
	if got := pickDisplayName("Alice", "ali", "123"); got != "Alice" {
		t.Fatalf("expected display name, got %q", got)
	}
	if got := pickDisplayName("", "Bob", "123"); got != "Bob" {
		t.Fatalf("expected profile name, got %q", got)
	}
	if got := pickDisplayName("", "", "123"); got != "123" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestAggregatePermissionState(t *testing.T) {
	perms := Permissions{Contacts: true, Templates: true, ConversationSync: true}
	if aggregatePermissionState(perms, false) != PermissionGranted {
		t.Fatal("expected granted")
	}
	if aggregatePermissionState(perms, true) != PermissionExpired {
		t.Fatal("expected expired")
	}
	perms = Permissions{Contacts: false, Templates: true, ConversationSync: false}
	if aggregatePermissionState(perms, false) != PermissionMissing {
		t.Fatal("expected missing")
	}
}
