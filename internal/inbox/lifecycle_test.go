package inbox_test

import (
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
)

func TestNextStatusOnInbound(t *testing.T) {
	cases := []struct {
		in   conversation.Status
		want conversation.Status
	}{
		{conversation.StatusOpen, conversation.StatusOpen},
		{conversation.StatusPending, conversation.StatusPending},
		{conversation.StatusResolved, conversation.StatusOpen},
		{conversation.StatusClosed, conversation.StatusOpen},
	}
	for _, tc := range cases {
		if got := inbox.NextStatusOnInbound(tc.in); got != tc.want {
			t.Fatalf("%s -> %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestShouldReopen(t *testing.T) {
	if !inbox.ShouldReopen(conversation.StatusClosed) {
		t.Fatal("closed should reopen")
	}
	if inbox.ShouldReopen(conversation.StatusOpen) {
		t.Fatal("open should not reopen")
	}
}
