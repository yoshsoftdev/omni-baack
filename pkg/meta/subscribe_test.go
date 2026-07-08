package meta_test

import (
	"testing"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestIsSubscribeAlreadyDone(t *testing.T) {
	if !metapkg.IsSubscribeAlreadyDone(metapkg.NewAPIError(400, `{"error":{"message":"App is already subscribed"}}`)) {
		t.Fatal("expected already subscribed to be treated as done")
	}
	if metapkg.IsSubscribeAlreadyDone(metapkg.NewAPIError(400, `{"error":{"message":"permissions error"}}`)) {
		t.Fatal("expected unrelated error to remain a failure")
	}
}

func TestIsAlreadySubscribed(t *testing.T) {
	if !metapkg.IsAlreadySubscribed(400, "duplicate subscription") {
		t.Fatal("expected duplicate subscription to be recognized")
	}
}
