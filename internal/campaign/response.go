package campaign

import (
	"encoding/json"
	"time"
)

// CampaignResponse is the API v1 wire format for campaign list/detail.
type CampaignResponse struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Description      *string          `json:"description,omitempty"`
	Status           string           `json:"status"`
	ChannelID        string           `json:"channel_id"`
	TemplateID       string           `json:"template_id"`
	TemplateName     string           `json:"template_name,omitempty"`
	TemplateLanguage string           `json:"template_language,omitempty"`
	ScheduledAt      *time.Time       `json:"scheduled_at,omitempty"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	TotalContacts    int              `json:"total_contacts"`
	SentCount        int              `json:"sent_count"`
	DeliveredCount   int              `json:"delivered_count"`
	ReadCount        int              `json:"read_count"`
	FailedCount      int              `json:"failed_count"`
	ReplyCount       int              `json:"reply_count"`
	ClickedCount     int              `json:"clicked_count"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Audience         *AudienceSummary `json:"audience,omitempty"`
	Audiences        []AudienceSourceView `json:"audiences,omitempty"`
}

func NewCampaignResponse(c Campaign, enrichment ListEnrichment) CampaignResponse {
	audience := BuildAudienceSummary(c, enrichment)
	resp := CampaignResponse{
		ID:               c.ID.String(),
		Name:             c.Name,
		Description:      c.Description,
		Status:           string(c.Status),
		ChannelID:        c.ChannelID.String(),
		TemplateID:       c.TemplateID.String(),
		TemplateName:     enrichment.TemplateName,
		TemplateLanguage: enrichment.TemplateLanguage,
		ScheduledAt:      c.ScheduledAt,
		StartedAt:        c.StartedAt,
		CompletedAt:      c.CompletedAt,
		TotalContacts:    c.TotalContacts,
		SentCount:        c.SentCount,
		DeliveredCount:   c.DeliveredCount,
		ReadCount:        c.ReadCount,
		FailedCount:      c.FailedCount,
		ReplyCount:       c.ReplyCount,
		ClickedCount:     c.ClickedCount,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
		Audience:         &audience,
		Audiences:        audience.Sources,
	}
	if audience.Type == "" && len(audience.Sources) == 0 {
		resp.Audience = nil
	}
	return resp
}

func MapVariableMapping(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}
