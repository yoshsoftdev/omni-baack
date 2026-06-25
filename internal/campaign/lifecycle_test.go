package campaign

import "testing"

func TestCanTransitionCampaignStatus(t *testing.T) {
	allowed := []struct {
		from, to Status
	}{
		{StatusDraft, StatusScheduled},
		{StatusDraft, StatusQueued},
		{StatusDraft, StatusCancelled},
		{StatusScheduled, StatusQueued},
		{StatusScheduled, StatusCancelled},
		{StatusQueued, StatusSending},
		{StatusQueued, StatusCancelled},
		{StatusSending, StatusCompleted},
		{StatusSending, StatusFailed},
		{StatusSending, StatusCancelled},
	}
	for _, tc := range allowed {
		if !CanTransitionCampaignStatus(tc.from, tc.to) {
			t.Fatalf("expected %s -> %s to be allowed", tc.from, tc.to)
		}
	}

	illegal := []struct {
		from, to Status
	}{
		{StatusDraft, StatusCompleted},
		{StatusDraft, StatusSending},
		{StatusQueued, StatusCompleted},
		{StatusCompleted, StatusCancelled},
		{StatusCancelled, StatusQueued},
		{StatusFailed, StatusQueued},
	}
	for _, tc := range illegal {
		if CanTransitionCampaignStatus(tc.from, tc.to) {
			t.Fatalf("expected %s -> %s to be rejected", tc.from, tc.to)
		}
	}
}

func TestCanCancelRecipient(t *testing.T) {
	if !CanCancelRecipient(RecipientQueued, false) {
		t.Fatal("queued should be cancellable")
	}
	if !CanCancelRecipient(RecipientSending, false) {
		t.Fatal("sending without wamid should be cancellable")
	}
	if CanCancelRecipient(RecipientSending, true) {
		t.Fatal("sending with wamid must not be cancellable")
	}
	if CanCancelRecipient(RecipientSent, true) {
		t.Fatal("sent must not be cancellable")
	}
	if CanCancelRecipient(RecipientDelivered, false) {
		t.Fatal("delivered must not be cancellable")
	}
}
