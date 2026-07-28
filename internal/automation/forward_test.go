package automation

import "testing"

func TestWebhookSubscribes(t *testing.T) {
	if !webhookSubscribes(nil, EventMessageReceived) {
		t.Fatal("empty events should default to message.received")
	}
	if !webhookSubscribes([]string{EventMessageReceived}, EventMessageReceived) {
		t.Fatal("expected subscribed")
	}
	if webhookSubscribes([]string{"other.event"}, EventMessageReceived) {
		t.Fatal("expected not subscribed")
	}
}

func TestNormalizeForwardEvents(t *testing.T) {
	got := normalizeForwardEvents([]string{" message.received ", "unknown", "message.received"})
	if len(got) != 1 || got[0] != EventMessageReceived {
		t.Fatalf("unexpected %#v", got)
	}
	got = normalizeForwardEvents(nil)
	if len(got) != 1 || got[0] != EventMessageReceived {
		t.Fatalf("unexpected default %#v", got)
	}
}
