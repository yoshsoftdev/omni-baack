package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

type mockCampaignRepo struct {
	created *Campaign
}

func (m *mockCampaignRepo) Create(ctx context.Context, campaign *Campaign, contactIDs, tagIDs []uuid.UUID) error {
	m.created = campaign
	return nil
}
func (m *mockCampaignRepo) FindByID(ctx context.Context, teamID, campaignID uuid.UUID) (*Campaign, error) {
	return m.created, nil
}
func (m *mockCampaignRepo) FindByIDOnly(ctx context.Context, campaignID uuid.UUID) (*Campaign, error) {
	return m.created, nil
}
func (m *mockCampaignRepo) ListByTeam(ctx context.Context, teamID uuid.UUID, limit int) ([]Campaign, error) {
	return nil, nil
}
func (m *mockCampaignRepo) Update(ctx context.Context, campaign *Campaign) error { return nil }
func (m *mockCampaignRepo) ListDueScheduled(ctx context.Context, now time.Time, limit int) ([]Campaign, error) {
	return nil, nil
}
func (m *mockCampaignRepo) ReplaceContacts(ctx context.Context, campaignID uuid.UUID, contacts []CampaignContact) error {
	return nil
}
func (m *mockCampaignRepo) ListPendingContacts(ctx context.Context, campaignID uuid.UUID) ([]CampaignContact, error) {
	return nil, nil
}
func (m *mockCampaignRepo) UpdateContact(ctx context.Context, contact *CampaignContact) error { return nil }
func (m *mockCampaignRepo) IncrementCounts(ctx context.Context, campaignID uuid.UUID, sent, failed int) error {
	return nil
}
func (m *mockCampaignRepo) FindContactByWamid(ctx context.Context, wamid string) (*CampaignContact, error) {
	return nil, errors.New("not found")
}
func (m *mockCampaignRepo) UpdateContactStatus(ctx context.Context, wamid string, status ContactStatus, at time.Time) error {
	return nil
}
func (m *mockCampaignRepo) RecalculateCounts(ctx context.Context, campaignID uuid.UUID) error { return nil }
func (m *mockCampaignRepo) ListTagIDs(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockCampaignRepo) ListAudienceContactIDs(ctx context.Context, campaignID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *mockCampaignRepo) ListContactsByCampaign(ctx context.Context, campaignID uuid.UUID) ([]CampaignContact, error) {
	return nil, nil
}

type mockContactRepo struct{}

func (m *mockContactRepo) Create(ctx context.Context, contact *contact.Contact) error { return nil }
func (m *mockContactRepo) FindByID(ctx context.Context, teamID, contactID uuid.UUID) (*contact.Contact, error) {
	return nil, contact.ErrNotFound
}
func (m *mockContactRepo) FindByPhone(ctx context.Context, teamID uuid.UUID, phone string) (*contact.Contact, error) {
	return nil, contact.ErrNotFound
}
func (m *mockContactRepo) ListByTeam(ctx context.Context, teamID uuid.UUID, filter contact.ListFilter) ([]contact.Contact, error) {
	return nil, nil
}
func (m *mockContactRepo) Update(ctx context.Context, contact *contact.Contact) error { return nil }
func (m *mockContactRepo) SoftDelete(ctx context.Context, teamID, contactID uuid.UUID) error { return nil }
func (m *mockContactRepo) ListTags(ctx context.Context, contactID uuid.UUID) ([]tag.Tag, error) {
	return nil, nil
}
func (m *mockContactRepo) SetTags(ctx context.Context, contactID uuid.UUID, tagIDs []uuid.UUID) error {
	return nil
}
func (m *mockContactRepo) ListByTagIDs(ctx context.Context, teamID uuid.UUID, tagIDs []uuid.UUID) ([]contact.Contact, error) {
	return nil, nil
}
func (m *mockContactRepo) ListByIDs(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]contact.Contact, error) {
	return nil, nil
}

type mockDeviceRepo struct{}

func (m *mockDeviceRepo) Create(ctx context.Context, device *device.Device) error { return nil }
func (m *mockDeviceRepo) FindByID(ctx context.Context, teamID, deviceID uuid.UUID) (*device.Device, error) {
	return &device.Device{ID: deviceID, TeamID: teamID}, nil
}
func (m *mockDeviceRepo) FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*device.Device, error) {
	return nil, device.ErrNotFound
}
func (m *mockDeviceRepo) FindBySlug(ctx context.Context, slug string) (*device.Device, error) {
	return nil, device.ErrNotFound
}
func (m *mockDeviceRepo) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]device.Device, error) {
	return nil, nil
}
func (m *mockDeviceRepo) Update(ctx context.Context, device *device.Device) error { return nil }
func (m *mockDeviceRepo) SoftDelete(ctx context.Context, teamID, deviceID uuid.UUID) error { return nil }

type mockTemplateRepo struct{}

func (m *mockTemplateRepo) Upsert(ctx context.Context, tpl *template.Template) error { return nil }
func (m *mockTemplateRepo) FindByID(ctx context.Context, teamID, templateID uuid.UUID) (*template.Template, error) {
	return &template.Template{ID: templateID, TeamID: teamID, Name: "hello_world", Language: "en_US"}, nil
}
func (m *mockTemplateRepo) ListByTeam(ctx context.Context, teamID uuid.UUID, filter template.ListFilter) ([]template.Template, error) {
	return nil, nil
}

type mockTeamRepo struct{}

func (m *mockTeamRepo) Create(ctx context.Context, team *team.Team) error { return nil }
func (m *mockTeamRepo) FindByID(ctx context.Context, id uuid.UUID) (*team.Team, error) { return nil, nil }
func (m *mockTeamRepo) Update(ctx context.Context, team *team.Team) error { return nil }
func (m *mockTeamRepo) CreateMember(ctx context.Context, member *team.Member) error { return nil }
func (m *mockTeamRepo) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*team.Member, error) {
	return nil, nil
}
func (m *mockTeamRepo) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*team.Member, error) {
	return &team.Member{}, nil
}
func (m *mockTeamRepo) FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*team.Member, error) {
	return nil, nil
}
func (m *mockTeamRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (m *mockTeamRepo) UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role team.Role) error {
	return nil
}
func (m *mockTeamRepo) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error { return nil }

type mockQueue struct{}

func (m *mockQueue) Enqueue(ctx context.Context, campaignID uuid.UUID) error { return nil }
func (m *mockQueue) Dequeue(ctx context.Context) (uuid.UUID, error)          { return uuid.Nil, nil }

func TestCreateCampaignRequiresRecipients(t *testing.T) {
	svc := NewService(&mockCampaignRepo{}, &mockContactRepo{}, &mockDeviceRepo{}, &mockTemplateRepo{}, &mockTeamRepo{}, &mockQueue{})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(), team.RoleAdmin, CreateInput{
		Name:       "Promo",
		DeviceID:   uuid.New(),
		TemplateID: uuid.New(),
	})
	if !errors.Is(err, ErrRecipientsRequired) {
		t.Fatalf("expected ErrRecipientsRequired, got %v", err)
	}
}

func TestCreateCampaignDraft(t *testing.T) {
	repo := &mockCampaignRepo{}
	svc := NewService(repo, &mockContactRepo{}, &mockDeviceRepo{}, &mockTemplateRepo{}, &mockTeamRepo{}, &mockQueue{})
	teamID := uuid.New()
	resp, err := svc.Create(context.Background(), teamID, uuid.New(), team.RoleAdmin, CreateInput{
		Name:            "Promo",
		DeviceID:        uuid.New(),
		TemplateID:      uuid.New(),
		VariableMapping: json.RawMessage(`{"body":{"1":"name"}}`),
		ContactIDs:      []uuid.UUID{uuid.New()},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if resp.Status != StatusDraft {
		t.Fatalf("expected draft status, got %s", resp.Status)
	}
}
