package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/campaignqueue"
)

var (
	ErrForbidden          = errors.New("insufficient permissions")
	ErrNameRequired       = errors.New("campaign name is required")
	ErrRecipientsRequired = errors.New("at least one recipient is required")
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDeviceNotConnected = errors.New("device is not connected")
	ErrTemplateNotFound   = errors.New("template not found")
	ErrInvalidStatus      = errors.New("campaign cannot be sent in current status")
	ErrCannotCancel       = errors.New("campaign cannot be cancelled")
)

type Service struct {
	campaigns Repository
	contacts  contact.Repository
	devices   device.Repository
	templates template.Repository
	teams     team.Repository
	queue     campaignqueue.Queue
}

func NewService(
	campaigns Repository,
	contacts contact.Repository,
	devices device.Repository,
	templates template.Repository,
	teams team.Repository,
	queue campaignqueue.Queue,
) *Service {
	return &Service{
		campaigns: campaigns,
		contacts:  contacts,
		devices:   devices,
		templates: templates,
		teams:     teams,
		queue:     queue,
	}
}

type CreateInput struct {
	Name            string
	DeviceID        uuid.UUID
	TemplateID      uuid.UUID
	VariableMapping json.RawMessage
	ContactIDs      []uuid.UUID
	TagIDs          []uuid.UUID
	ScheduledAt     *time.Time
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID) ([]Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	items, err := s.campaigns.ListByTeam(ctx, teamID, 100)
	if err != nil {
		return nil, err
	}
	responses := make([]Response, 0, len(items))
	for i := range items {
		resp, err := s.enrichResponse(ctx, teamID, &items[i])
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

func (s *Service) GetOverview(ctx context.Context, teamID, userID uuid.UUID) (*OverviewResponse, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	items, err := s.campaigns.ListByTeam(ctx, teamID, 500)
	if err != nil {
		return nil, err
	}
	overview := &OverviewResponse{}
	var totalSent, totalDelivered, totalRead, totalFailed int
	for i := range items {
		c := items[i]
		overview.TotalCampaigns++
		totalSent += c.SentCount
		totalDelivered += c.DeliveredCount
		totalRead += c.ReadCount
		totalFailed += c.FailedCount
	}
	overview.TotalSent = totalSent
	if totalSent > 0 {
		overview.DeliveredRate = float64(totalDelivered) / float64(totalSent) * 100
		overview.ReadRate = float64(totalRead) / float64(totalSent) * 100
		overview.FailedRate = float64(totalFailed) / float64(totalSent) * 100
	}
	return overview, nil
}

func (s *Service) GetReport(ctx context.Context, teamID, userID, campaignID uuid.UUID) (*ReportResponse, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, err
	}
	resp, err := s.enrichResponse(ctx, teamID, item)
	if err != nil {
		return nil, err
	}
	report := ReportResponse{Response: resp}
	report.DeliveryBreakdown = buildDeliveryBreakdown(item)
	report.HourlyPerformance = buildHourlyPerformance(ctx, s.campaigns, campaignID)
	return &report, nil
}

func (s *Service) Cancel(ctx context.Context, teamID, userID uuid.UUID, role team.Role, campaignID uuid.UUID) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	campaign, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, err
	}
	if campaign.Status != StatusScheduled && campaign.Status != StatusDraft {
		return nil, ErrCannotCancel
	}
	now := time.Now().UTC()
	campaign.Status = StatusCancelled
	campaign.UpdatedAt = now
	if err := s.campaigns.Update(ctx, campaign); err != nil {
		return nil, err
	}
	resp, err := s.enrichResponse(ctx, teamID, campaign)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *Service) enrichResponse(ctx context.Context, teamID uuid.UUID, campaign *Campaign) (Response, error) {
	resp := ToResponse(campaign)
	dev, err := s.devices.FindByID(ctx, teamID, campaign.DeviceID)
	if err == nil {
		resp.DeviceName = &dev.Name
		resp.DevicePhone = dev.PhoneNumber
	}
	tpl, err := s.templates.FindByID(ctx, teamID, campaign.TemplateID)
	if err == nil {
		resp.TemplateName = &tpl.Name
		resp.TemplateLanguage = &tpl.Language
		resp.TemplateCategory = tpl.Category
	}
	return resp, nil
}

func buildDeliveryBreakdown(campaign *Campaign) []DeliveryBreakdown {
	return []DeliveryBreakdown{
		{Label: "Terkirim", Value: campaign.SentCount},
		{Label: "Terkirim (Delivered)", Value: campaign.DeliveredCount},
		{Label: "Terbaca", Value: campaign.ReadCount},
		{Label: "Gagal", Value: campaign.FailedCount},
	}
}

func buildHourlyPerformance(ctx context.Context, repo Repository, campaignID uuid.UUID) []HourlyPerformance {
	contacts, err := repo.ListContactsByCampaign(ctx, campaignID)
	if err != nil {
		return nil
	}
	buckets := make([]int, 24)
	for i := range contacts {
		if contacts[i].SentAt == nil {
			continue
		}
		buckets[contacts[i].SentAt.UTC().Hour()]++
	}
	result := make([]HourlyPerformance, 24)
	for hour := 0; hour < 24; hour++ {
		result[hour] = HourlyPerformance{Hour: hour, Count: buckets[hour]}
	}
	return result
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, role team.Role, input CreateInput) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	if input.DeviceID == uuid.Nil || input.TemplateID == uuid.Nil {
		return nil, errors.New("device_id and template_id are required")
	}
	if len(input.ContactIDs) == 0 && len(input.TagIDs) == 0 {
		return nil, ErrRecipientsRequired
	}

	if _, err := s.devices.FindByID(ctx, teamID, input.DeviceID); err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	if _, err := s.templates.FindByID(ctx, teamID, input.TemplateID); err != nil {
		if errors.Is(err, template.ErrNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}

	mapping := input.VariableMapping
	if len(mapping) == 0 {
		mapping = json.RawMessage(`{}`)
	}
	if _, err := parseVariableMapping(mapping); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	status := StatusDraft
	if input.ScheduledAt != nil && input.ScheduledAt.After(now) {
		status = StatusScheduled
	}

	campaign := &Campaign{
		ID:              uuid.New(),
		TeamID:          teamID,
		DeviceID:        input.DeviceID,
		TemplateID:      input.TemplateID,
		Name:            name,
		Status:          status,
		VariableMapping: mapping,
		ScheduledAt:     input.ScheduledAt,
		CreatedBy:       userID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.campaigns.Create(ctx, campaign, input.ContactIDs, input.TagIDs); err != nil {
		return nil, err
	}

	resp, err := s.enrichResponse(ctx, teamID, campaign)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *Service) Send(ctx context.Context, teamID, userID uuid.UUID, role team.Role, campaignID uuid.UUID, contactIDs []uuid.UUID) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	campaign, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, err
	}
	if campaign.Status != StatusDraft && campaign.Status != StatusScheduled {
		return nil, ErrInvalidStatus
	}

	tagIDs, err := s.campaigns.ListTagIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if len(contactIDs) == 0 {
		contactIDs, err = s.campaigns.ListAudienceContactIDs(ctx, campaignID)
		if err != nil {
			return nil, err
		}
	}

	if err := s.prepareAndQueue(ctx, campaign, contactIDs, tagIDs); err != nil {
		return nil, err
	}

	updated, err := s.campaigns.FindByID(ctx, teamID, campaignID)
	if err != nil {
		return nil, err
	}
	resp, err := s.enrichResponse(ctx, teamID, updated)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *Service) prepareAndQueue(ctx context.Context, campaign *Campaign, contactIDs, tagIDs []uuid.UUID) error {
	audience, err := s.resolveAudience(ctx, campaign.TeamID, contactIDs, tagIDs)
	if err != nil {
		return err
	}
	if len(audience) == 0 {
		return ErrRecipientsRequired
	}

	now := time.Now().UTC()
	contacts := make([]CampaignContact, 0, len(audience))
	for i := range audience {
		c := audience[i]
		contactID := c.ID
		contacts = append(contacts, CampaignContact{
			ID:         uuid.New(),
			CampaignID: campaign.ID,
			ContactID:  &contactID,
			Phone:      c.Phone,
			Status:     ContactPending,
			CreatedAt:  now,
		})
	}
	if err := s.campaigns.ReplaceContacts(ctx, campaign.ID, contacts); err != nil {
		return err
	}

	campaign.TotalContacts = len(contacts)
	campaign.Status = StatusQueued
	campaign.UpdatedAt = now
	if err := s.campaigns.Update(ctx, campaign); err != nil {
		return err
	}
	return s.queue.Enqueue(ctx, campaign.ID)
}

func (s *Service) resolveAudience(ctx context.Context, teamID uuid.UUID, contactIDs, tagIDs []uuid.UUID) ([]contact.Contact, error) {
	seen := make(map[string]contact.Contact)

	if len(contactIDs) > 0 {
		items, err := s.contacts.ListByIDs(ctx, teamID, contactIDs)
		if err != nil {
			return nil, err
		}
		for i := range items {
			seen[items[i].Phone] = items[i]
		}
	}
	if len(tagIDs) > 0 {
		items, err := s.contacts.ListByTagIDs(ctx, teamID, tagIDs)
		if err != nil {
			return nil, err
		}
		for i := range items {
			seen[items[i].Phone] = items[i]
		}
	}

	result := make([]contact.Contact, 0, len(seen))
	for _, c := range seen {
		result = append(result, c)
	}
	return result, nil
}

func (s *Service) HandleDeliveryStatus(ctx context.Context, wamid, status string, at time.Time) error {
	var contactStatus ContactStatus
	switch status {
	case "delivered":
		contactStatus = ContactDelivered
	case "read":
		contactStatus = ContactRead
	default:
		return nil
	}
	if err := s.campaigns.UpdateContactStatus(ctx, wamid, contactStatus, at); err != nil {
		return err
	}
	cc, err := s.campaigns.FindContactByWamid(ctx, wamid)
	if err != nil {
		return nil
	}
	return s.campaigns.RecalculateCounts(ctx, cc.CampaignID)
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}
