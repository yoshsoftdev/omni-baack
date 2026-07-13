package inbox

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

func TestAgeBonus_Buckets(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		minutes int
		want    int
	}{
		{0, 0},
		{1, 10},
		{2, 20},
		{5, 50},
		{15, 100},
		{20, 150},
	}
	for _, tc := range cases {
		since := base.Add(-time.Duration(tc.minutes) * time.Minute)
		got := AgeBonus(&since, base)
		if got != tc.want {
			t.Fatalf("minutes=%d got=%d want=%d", tc.minutes, got, tc.want)
		}
	}
}

func TestRecalculatePriority_InboundSetsWaiting(t *testing.T) {
	now := time.Now().UTC()
	conv := &conversation.Conversation{
		ID:          uuid.New(),
		Status:      conversation.StatusOpen,
		UnreadCount: 1,
	}
	upd := RecalculatePriority(conv, PriorityEventInbound, now)
	if upd.WaitingSince == nil {
		t.Fatal("expected waiting_since")
	}
	if upd.BasePriority < 10 {
		t.Fatalf("base_priority=%d", upd.BasePriority)
	}
	if upd.PriorityScore < int64(upd.BasePriority) {
		t.Fatalf("priority_score=%d base=%d", upd.PriorityScore, upd.BasePriority)
	}
}

func TestRecalculatePriority_OutboundClearsWaiting(t *testing.T) {
	now := time.Now().UTC()
	since := now.Add(-10 * time.Minute)
	conv := &conversation.Conversation{
		Status:       conversation.StatusOpen,
		WaitingSince: &since,
		UnreadCount:  2,
	}
	upd := RecalculatePriority(conv, PriorityEventOutbound, now)
	if upd.WaitingSince != nil {
		t.Fatal("expected waiting_since cleared")
	}
	if upd.PriorityScore != int64(upd.BasePriority) {
		t.Fatalf("score=%d base=%d", upd.PriorityScore, upd.BasePriority)
	}
}

func TestComputeBasePriority_UnassignedOpen(t *testing.T) {
	conv := &conversation.Conversation{Status: conversation.StatusOpen, UnreadCount: 3}
	base := ComputeBasePriority(conv)
	if base != 130 { // 30 unread + 100 unassigned
		t.Fatalf("base=%d want 130", base)
	}
}

func TestComputeBasePriority_Closed(t *testing.T) {
	conv := &conversation.Conversation{Status: conversation.StatusClosed}
	if ComputeBasePriority(conv) != -200 {
		t.Fatal("closed should lower base priority")
	}
}
