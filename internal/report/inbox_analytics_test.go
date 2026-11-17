package report

import (
	"testing"
	"time"
)

// Days are calendar days in the viewer's zone, filled with zeros where nothing happened.
func TestFillDaysUsesViewerTimeZone(t *testing.T) {
	jakarta, _ := time.LoadLocation("Asia/Jakarta") // UTC+7
	q := InboxAnalyticsQuery{
		// 2026-11-01 17:00 UTC is already 2026-11-02 00:00 in Jakarta.
		From:     time.Date(2026, 11, 1, 17, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 11, 4, 17, 0, 0, 0, time.UTC),
		Location: jakarta,
	}
	got := fillDays([]InboxDay{{Date: "2026-11-03", MessagesReceived: 5}}, q)
	want := []string{"2026-11-02", "2026-11-03", "2026-11-04"}
	if len(got) != len(want) {
		t.Fatalf("days = %+v, want %v", got, want)
	}
	for i, d := range got {
		if d.Date != want[i] {
			t.Fatalf("day %d = %s, want %s", i, d.Date, want[i])
		}
	}
	if got[1].MessagesReceived != 5 || got[0].MessagesReceived != 0 {
		t.Fatalf("values not kept/zeroed: %+v", got)
	}
}
