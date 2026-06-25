package meta_test

import (
	"testing"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestVerifyWebhookSubscriptionValid(t *testing.T) {
	challenge, err := metapkg.VerifyWebhookSubscription("subscribe", "secret", "12345", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if challenge != "12345" {
		t.Fatalf("challenge = %q", challenge)
	}
}

func TestVerifyWebhookSubscriptionInvalidToken(t *testing.T) {
	_, err := metapkg.VerifyWebhookSubscription("subscribe", "wrong", "12345", "secret")
	if err == nil {
		t.Fatal("expected error")
	}
}
