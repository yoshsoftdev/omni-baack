package template

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var variablePattern = regexp.MustCompile(`\{\{\d+\}\}`)

// MapFromMeta converts a Meta API template into the domain template model.
func MapFromMeta(
	channelID, teamID uuid.UUID,
	wabaID string,
	meta metapkg.MessageTemplate,
	syncedAt time.Time,
) *Template {
	status := normalizeMetaStatus(meta.Status)
	language := normalizeMetaLanguage(meta.Language)
	components := meta.Components
	if len(components) == 0 {
		components = json.RawMessage("[]")
	}
	preview := extractBodyPreview(components)
	category := strings.TrimSpace(meta.Category)
	var categoryPtr *string
	if category != "" {
		categoryPtr = &category
	}

	return &Template{
		ID:             uuid.New(),
		ChannelID:      channelID,
		TeamID:         teamID,
		WABAID:         wabaID,
		MetaTemplateID: meta.ID,
		Name:           meta.Name,
		Language:       language,
		Category:       categoryPtr,
		Status:         status,
		Preview:        preview,
		Components:     components,
		VariableCount:  countVariables(components),
		LastSyncAt:     &syncedAt,
		CreatedAt:      syncedAt,
		UpdatedAt:      syncedAt,
	}
}

func normalizeMetaStatus(raw string) Status {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case string(StatusApproved):
		return StatusApproved
	case string(StatusRejected):
		return StatusRejected
	default:
		return StatusPending
	}
}

func normalizeMetaLanguage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "en_US"
	}
	parts := strings.Split(raw, "_")
	if len(parts) == 1 {
		return strings.ToLower(parts[0]) + "_" + strings.ToUpper(parts[0])
	}
	return strings.ToLower(parts[0]) + "_" + strings.ToUpper(parts[1])
}

func extractBodyPreview(components json.RawMessage) *string {
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(components, &items); err != nil {
		return nil
	}
	for _, item := range items {
		if strings.EqualFold(item.Type, "BODY") && strings.TrimSpace(item.Text) != "" {
			text := strings.TrimSpace(item.Text)
			return &text
		}
	}
	return nil
}

func countVariables(components json.RawMessage) int {
	return len(variablePattern.FindAllString(string(components), -1))
}
