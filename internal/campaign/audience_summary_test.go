package campaign

import "testing"

func TestBuildAudienceSummaryMixed(t *testing.T) {
	c := Campaign{Status: StatusDraft, TotalContacts: 0}
	summary := BuildAudienceSummary(c, ListEnrichment{
		Sources: []AudienceSourceView{
			{Type: string(AudienceSegment), Name: "VIP Customers"},
			{Type: string(AudienceTag), Name: "Reseller"},
		},
	})
	if summary.Type != "mixed" {
		t.Fatalf("type = %q, want mixed", summary.Type)
	}
	if summary.Name != "VIP Customers" {
		t.Fatalf("name = %q", summary.Name)
	}
	if summary.ExtraAudiences != 1 {
		t.Fatalf("extra = %d, want 1", summary.ExtraAudiences)
	}
}

func TestBuildAudienceSummarySnapshot(t *testing.T) {
	c := Campaign{Status: StatusScheduled, TotalContacts: 1284}
	summary := BuildAudienceSummary(c, ListEnrichment{
		Sources:     []AudienceSourceView{{Type: string(AudienceSegment), Name: "VIP Customers"}},
		HasSnapshot: true,
	})
	if !summary.Snapshot {
		t.Fatalf("expected snapshot=true")
	}
	if summary.RecipientCount != 1284 {
		t.Fatalf("recipient_count = %d", summary.RecipientCount)
	}
}
