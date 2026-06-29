package campaign

import (
	"strings"
	"time"
)

// AudienceSourceView is a resolved audience source with display name.
type AudienceSourceView struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AudienceSummary is the list/detail audience block for API responses.
type AudienceSummary struct {
	Type           string               `json:"type"`
	Name           string               `json:"name"`
	RecipientCount int                  `json:"recipient_count"`
	Snapshot       bool                 `json:"snapshot"`
	SnapshotAt     *time.Time           `json:"snapshot_at,omitempty"`
	ExtraAudiences int                  `json:"extra_audiences,omitempty"`
	Sources        []AudienceSourceView `json:"sources,omitempty"`
}

// ListEnrichment carries joined list-row metadata for a campaign.
type ListEnrichment struct {
	TemplateName     string
	TemplateLanguage string
	Sources          []AudienceSourceView
	SnapshotAt       *time.Time
	SnapshotResolved int
	HasSnapshot      bool
}

func BuildAudienceSummary(c Campaign, enrichment ListEnrichment) AudienceSummary {
	sources := enrichment.Sources
	recipientCount := c.TotalContacts
	if recipientCount == 0 && enrichment.SnapshotResolved > 0 {
		recipientCount = enrichment.SnapshotResolved
	}

	summary := AudienceSummary{
		RecipientCount: recipientCount,
		Sources:        sources,
	}

	if len(sources) == 0 {
		summary.Type = ""
		summary.Name = ""
		return summary
	}

	if len(sources) == 1 {
		summary.Type = mapSourceType(sources[0].Type)
		summary.Name = sources[0].Name
	} else {
		contactOnly := true
		for _, src := range sources {
			if src.Type != string(AudienceContact) {
				contactOnly = false
				break
			}
		}
		if contactOnly {
			summary.Type = "manual"
			summary.Name = "Manual"
		} else {
			summary.Type = "mixed"
			summary.Name = sources[0].Name
			summary.ExtraAudiences = len(sources) - 1
		}
	}

	summary.Snapshot = usesSnapshot(c.Status, recipientCount, enrichment)
	if summary.Snapshot && enrichment.SnapshotAt != nil {
		summary.SnapshotAt = enrichment.SnapshotAt
	}

	return summary
}

func mapSourceType(sourceType string) string {
	switch AudienceSourceType(sourceType) {
	case AudienceSegment:
		return "segment"
	case AudienceTag:
		return "tag"
	case AudienceContact:
		return "manual"
	default:
		return strings.ToLower(strings.TrimSpace(sourceType))
	}
}

func usesSnapshot(status Status, recipientCount int, enrichment ListEnrichment) bool {
	if enrichment.HasSnapshot && recipientCount > 0 {
		return true
	}
	switch status {
	case StatusScheduled, StatusQueued, StatusSending, StatusCompleted, StatusFailed, StatusCancelled:
		return recipientCount > 0 || enrichment.SnapshotResolved > 0
	default:
		return false
	}
}
