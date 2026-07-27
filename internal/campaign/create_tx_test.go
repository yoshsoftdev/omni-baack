package campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/recipient"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/segment"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)


type trackingTxManager struct {
	runCount int
	repo     Repository
}

func (m *trackingTxManager) Run(ctx context.Context, fn func(context.Context, Repository) error) error {
	m.runCount++
	return fn(ctx, m.repo)
}

type spyCampaignRepo struct {
	createCalls     int
	jobCalls        int
	snapCalls       int
	recipientCalls  int
	softDeleteCalls int
	snapErr         error
}

func (s *spyCampaignRepo) Create(context.Context, *Campaign, []Audience) error {
	s.createCalls++
	return nil
}
func (s *spyCampaignRepo) CreateJob(context.Context, *BroadcastJob) error {
	s.jobCalls++
	return nil
}
func (s *spyCampaignRepo) CreateAudienceSnapshot(context.Context, *AudienceSnapshot) error {
	s.snapCalls++
	return s.snapErr
}
func (s *spyCampaignRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error {
	s.softDeleteCalls++
	return nil
}

func (s *spyCampaignRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*Campaign, error) {
	return nil, repository.ErrNotFound
}
func (s *spyCampaignRepo) List(context.Context, uuid.UUID, ListFilter, repository.PageParams) (repository.Page[Campaign], error) {
	return repository.Page[Campaign]{}, nil
}
func (s *spyCampaignRepo) Update(context.Context, *Campaign) error { return nil }
func (s *spyCampaignRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, Status) error {
	return nil
}
func (s *spyCampaignRepo) ListAudiences(context.Context, uuid.UUID) ([]Audience, error) {
	return nil, nil
}

func (s *spyCampaignRepo) LoadListEnrichment(context.Context, uuid.UUID, []Campaign) (map[uuid.UUID]ListEnrichment, error) {
	return map[uuid.UUID]ListEnrichment{}, nil
}
func (s *spyCampaignRepo) UpdateJob(context.Context, *BroadcastJob) error { return nil }
func (s *spyCampaignRepo) FindActiveJobByCampaign(context.Context, uuid.UUID) (*BroadcastJob, error) {
	return nil, repository.ErrNotFound
}
func (s *spyCampaignRepo) CreateBatch(context.Context, *BroadcastBatch) error { return nil }
func (s *spyCampaignRepo) UpdateBatch(context.Context, *BroadcastBatch) error { return nil }
func (s *spyCampaignRepo) ListBatchesByJob(context.Context, uuid.UUID) ([]BroadcastBatch, error) {
	return nil, nil
}
func (s *spyCampaignRepo) BulkCreateRecipients(context.Context, []Recipient, time.Time) error {
	s.recipientCalls++
	return nil
}
func (s *spyCampaignRepo) UpdateCampaignTotalContacts(context.Context, uuid.UUID, int) error {
	return nil
}
func (s *spyCampaignRepo) UpdateRecipientStatus(context.Context, uuid.UUID, RecipientStatus, RecipientStatus, *string, *string) error {
	return nil
}
func (s *spyCampaignRepo) FindRecipientByWAMID(context.Context, string) (*Recipient, error) {
	return nil, repository.ErrNotFound
}
func (s *spyCampaignRepo) ApplyCounterDelta(context.Context, uuid.UUID, CounterDelta) error {
	return nil
}
func (s *spyCampaignRepo) UpdateRecipientDelivery(context.Context, uuid.UUID, RecipientStatus, RecipientStatus, RecipientDeliveryUpdate) error {
	return nil
}
func (s *spyCampaignRepo) FindRecipientByWAMIDForTeam(context.Context, uuid.UUID, string) (*Recipient, error) {
	return nil, repository.ErrNotFound
}
func (s *spyCampaignRepo) CountNonTerminalRecipients(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (s *spyCampaignRepo) CompleteCampaign(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (s *spyCampaignRepo) AggregateRecipientStatusCounts(context.Context, uuid.UUID) (RecipientStatusCounts, error) {
	return RecipientStatusCounts{}, nil
}
func (s *spyCampaignRepo) SetCampaignCounters(context.Context, uuid.UUID, RecipientStatusCounts) error {
	return nil
}
func (s *spyCampaignRepo) RepairCampaignCounters(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *spyCampaignRepo) ListCampaignIDsForReconciliation(context.Context, time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (s *spyCampaignRepo) ClaimDueScheduled(context.Context, time.Time, int) ([]Campaign, error) {
	return nil, nil
}
func (s *spyCampaignRepo) TransitionCampaignToQueued(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *spyCampaignRepo) ListPendingJobs(context.Context, int) ([]BroadcastJob, error) {
	return nil, nil
}
func (s *spyCampaignRepo) FindJobByID(context.Context, uuid.UUID) (*BroadcastJob, error) {
	return nil, repository.ErrNotFound
}
func (s *spyCampaignRepo) CountRecipientsByJob(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (s *spyCampaignRepo) CountBatchesByJob(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (s *spyCampaignRepo) PlanBatchesForJob(context.Context, uuid.UUID, int) ([]BroadcastBatch, error) {
	return nil, nil
}
func (s *spyCampaignRepo) MarkBatchesQueued(context.Context, []uuid.UUID) error { return nil }
func (s *spyCampaignRepo) ListStaleProcessingBatches(context.Context, time.Time, int) ([]BroadcastBatch, error) {
	return nil, nil
}
func (s *spyCampaignRepo) RequeueStaleBatch(context.Context, uuid.UUID) error { return nil }
func (s *spyCampaignRepo) ResetSendingRecipientsWithoutWAMID(context.Context, uuid.UUID) error {
	return nil
}
func (s *spyCampaignRepo) InsertOutbox(context.Context, string, []byte) error { return nil }
func (s *spyCampaignRepo) ListPendingOutbox(context.Context, int) ([]OutboxMessage, error) {
	return nil, nil
}
func (s *spyCampaignRepo) MarkOutboxPublished(context.Context, uuid.UUID) error { return nil }
func (s *spyCampaignRepo) TransitionCampaignToQueuedForSend(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *spyCampaignRepo) CancelCampaignExecution(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

type createTestTeamRepo struct {
	member *team.Member
}

func (m *createTestTeamRepo) FindMemberByUser(context.Context, uuid.UUID, uuid.UUID) (*team.Member, error) {
	if m.member == nil {
		return nil, team.ErrMemberNotFound
	}
	return m.member, nil
}

func (m *createTestTeamRepo) Create(context.Context, *team.Team) error { return nil }
func (m *createTestTeamRepo) FindByID(context.Context, uuid.UUID) (*team.Team, error) {
	return nil, team.ErrNotFound
}
func (m *createTestTeamRepo) Update(context.Context, *team.Team) error { return nil }
func (m *createTestTeamRepo) CreateMember(context.Context, *team.Member) error { return nil }
func (m *createTestTeamRepo) FindPrimaryMembership(context.Context, uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (m *createTestTeamRepo) FindMemberByID(context.Context, uuid.UUID, uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (m *createTestTeamRepo) ListMembers(context.Context, uuid.UUID) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (m *createTestTeamRepo) ListMembersFiltered(context.Context, uuid.UUID, team.ListMembersFilter) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (m *createTestTeamRepo) UpdateMemberRole(context.Context, uuid.UUID, uuid.UUID, team.Role, uuid.UUID) error {
	return nil
}
func (m *createTestTeamRepo) UpdateMemberStatus(context.Context, uuid.UUID, uuid.UUID, team.MemberStatus) error {
	return nil
}
func (m *createTestTeamRepo) SoftDeleteMember(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *createTestTeamRepo) FindRoleIDByName(context.Context, string) (uuid.UUID, error) {
	return uuid.Nil, team.ErrNotFound
}
func (m *createTestTeamRepo) GetWorkspaceDetail(context.Context, uuid.UUID) (*team.WorkspaceDetail, error) {
	return nil, team.ErrNotFound
}
func (m *createTestTeamRepo) CreateInvitation(context.Context, *team.Invitation) error { return nil }
func (m *createTestTeamRepo) ListInvitations(context.Context, uuid.UUID) ([]team.Invitation, error) {
	return nil, nil
}

func (m *createTestTeamRepo) FindInvitationByID(context.Context, uuid.UUID, uuid.UUID) (*team.Invitation, error) {
	return nil, team.ErrNotFound
}

func (m *createTestTeamRepo) SoftDeleteInvitation(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (m *createTestTeamRepo) UpdateInvitationRole(context.Context, uuid.UUID, uuid.UUID, team.Role, uuid.UUID) error {
	return nil
}

func (m *createTestTeamRepo) GetPendingInvitation(context.Context, uuid.UUID) (*team.InvitationDetail, error) {
	return nil, team.ErrNotFound
}

func (m *createTestTeamRepo) MarkInvitationAccepted(context.Context, uuid.UUID) error { return nil }

type createTestChannelRepo struct {
	channelID uuid.UUID
	teamID    uuid.UUID
}

func (m *createTestChannelRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*channel.Channel, error) {
	if teamID != m.teamID || id != m.channelID {
		return nil, repository.ErrNotFound
	}
	return &channel.Channel{
		ID:          id,
		TeamID:      teamID,
		Status:      channel.StatusConnected,
		ChannelType: channel.TypeWhatsApp,
	}, nil
}
func (m *createTestChannelRepo) FindOnboarding(_ context.Context, channelID uuid.UUID) (*channel.Onboarding, error) {
	if channelID != m.channelID {
		return nil, repository.ErrNotFound
	}
	return &channel.Onboarding{ChannelID: channelID, Status: channel.OnboardingCompleted}, nil
}

func (m *createTestChannelRepo) Create(context.Context, *channel.Channel) error { return nil }
func (m *createTestChannelRepo) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	return repository.Page[channel.Channel]{}, nil
}
func (m *createTestChannelRepo) Update(context.Context, *channel.Channel) error { return nil }
func (m *createTestChannelRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *createTestChannelRepo) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (m *createTestChannelRepo) UpsertCredentials(context.Context, *channel.Credentials) error {
	return nil
}
func (m *createTestChannelRepo) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestChannelRepo) DeleteCredentials(context.Context, uuid.UUID) error { return nil }
func (m *createTestChannelRepo) UpsertOnboarding(context.Context, *channel.Onboarding) error {
	return nil
}
func (m *createTestChannelRepo) CountMetaReady(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (m *createTestChannelRepo) FindByPhoneNumberID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestChannelRepo) FindByWABAID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestChannelRepo) FindByIGUserID(context.Context, uuid.UUID, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestChannelRepo) FindByIGUserIDGlobal(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestChannelRepo) FindConnectedByType(context.Context, uuid.UUID, channel.ChannelType) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestChannelRepo) UpdateHealthMetadata(context.Context, uuid.UUID, []byte) error { return nil }

type createTestTemplateRepo struct {
	teamID     uuid.UUID
	channelID  uuid.UUID
	templateID uuid.UUID
}

func (m *createTestTemplateRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*template.Template, error) {
	if teamID != m.teamID || id != m.templateID {
		return nil, repository.ErrNotFound
	}
	return &template.Template{
		ID:        id,
		TeamID:    teamID,
		ChannelID: m.channelID,
		Name:      "welcome",
		Status:    template.StatusApproved,
	}, nil
}

func (m *createTestTemplateRepo) FindByIDs(_ context.Context, teamID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*template.Template, error) {
	out := make(map[uuid.UUID]*template.Template, len(ids))
	for _, id := range ids {
		tmpl, err := m.FindByID(context.Background(), teamID, id)
		if err == nil {
			out[id] = tmpl
		}
	}
	return out, nil
}

func (m *createTestTemplateRepo) FindByName(context.Context, uuid.UUID, uuid.UUID, string, string) (*template.Template, error) {
	return nil, repository.ErrNotFound
}

func (m *createTestTemplateRepo) FindByMetaID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*template.Template, error) {
	return m.FindByMetaTemplateID(ctx, teamID, channelID, metaTemplateID)
}

func (m *createTestTemplateRepo) Create(context.Context, *template.Template) error { return nil }
func (m *createTestTemplateRepo) Save(ctx context.Context, t *template.Template) error {
	return m.Create(ctx, t)
}
func (m *createTestTemplateRepo) Update(context.Context, *template.Template) error { return nil }
func (m *createTestTemplateRepo) Delete(ctx context.Context, teamID, id uuid.UUID) error {
	return m.SoftDelete(ctx, teamID, id)
}
func (m *createTestTemplateRepo) List(context.Context, uuid.UUID, template.ListFilter, repository.PageParams) (repository.Page[template.Template], error) {
	return repository.Page[template.Template]{}, nil
}
func (m *createTestTemplateRepo) UpsertFromSync(context.Context, *template.Template) error { return nil }
func (m *createTestTemplateRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *createTestTemplateRepo) SoftDeleteExceptMetaIDs(context.Context, uuid.UUID, uuid.UUID, []string) (int64, error) {
	return 0, nil
}
func (m *createTestTemplateRepo) FindByMetaTemplateID(context.Context, uuid.UUID, uuid.UUID, string) (*template.Template, error) {
	return nil, repository.ErrNotFound
}
func (m *createTestTemplateRepo) HasRecentSync(context.Context, uuid.UUID, time.Duration) (bool, error) {
	return false, nil
}
func (m *createTestTemplateRepo) UpdateFromWebhook(context.Context, uuid.UUID, uuid.UUID, string, string, template.Status, *string, time.Time) error {
	return nil
}

type createTestSegmentRepo struct{}

func (createTestSegmentRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*segment.Segment, []segment.Rule, error) {
	return nil, nil, repository.ErrNotFound
}
func (createTestSegmentRepo) Create(context.Context, *segment.Segment, []segment.Rule) error { return nil }
func (createTestSegmentRepo) List(context.Context, uuid.UUID, segment.ListFilter, repository.PageParams) (repository.Page[segment.Segment], error) {
	return repository.Page[segment.Segment]{}, nil
}
func (createTestSegmentRepo) Update(context.Context, *segment.Segment, []segment.Rule) error { return nil }
func (createTestSegmentRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (createTestSegmentRepo) ReplaceMembers(context.Context, uuid.UUID, []uuid.UUID) error { return nil }
func (createTestSegmentRepo) ListMemberContactIDs(context.Context, uuid.UUID, repository.PageParams) ([]uuid.UUID, int64, error) {
	return nil, 0, nil
}
func (createTestSegmentRepo) CountMembers(context.Context, uuid.UUID) (int64, error) { return 0, nil }

type createTestRbacRepo struct{}

func (createTestRbacRepo) LoadRolePermissions(context.Context) (map[string]map[string]bool, error) {
	return map[string]map[string]bool{
		"owner": {rbac.PermCampaignCreate: true},
	}, nil
}
func (createTestRbacRepo) ListRoles(context.Context) ([]rbac.Role, error) { return nil, nil }
func (createTestRbacRepo) FindRoleByID(context.Context, uuid.UUID) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (createTestRbacRepo) FindRoleByName(context.Context, string) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (createTestRbacRepo) ListPermissions(context.Context) ([]rbac.Permission, error) { return nil, nil }
func (createTestRbacRepo) ListPermissionsByRoleID(context.Context, uuid.UUID) ([]rbac.Permission, error) {
	return nil, nil
}
func (createTestRbacRepo) GetPermissionMatrix(context.Context) ([]rbac.PermissionMatrixRow, error) {
	return nil, nil
}
func (createTestRbacRepo) CreateRole(context.Context, *rbac.Role) error { return nil }
func (createTestRbacRepo) UpdateRole(context.Context, *rbac.Role) error { return nil }
func (createTestRbacRepo) SoftDeleteRole(context.Context, uuid.UUID) error { return nil }
func (createTestRbacRepo) ReplaceRolePermissions(context.Context, uuid.UUID, []string) error {
	return nil
}

func testCampaignRBAC(t *testing.T) *rbac.Service {
	t.Helper()
	svc := rbac.NewService(createTestRbacRepo{})
	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("WarmCache: %v", err)
	}
	return svc
}

func validCreateInput(teamID, channelID, templateID, contactID uuid.UUID) CreateInput {
	return CreateInput{
		Name:       "Summer Promo",
		ChannelID:  channelID,
		TemplateID: templateID,
		Audiences: []AudienceInput{
			{SourceType: AudienceContact, SourceID: contactID},
		},
		SendMode: SendNow,
	}
}

type createTestContactLookup struct {
	contacts map[uuid.UUID]contact.Contact
}

func (m *createTestContactLookup) FindByID(_ context.Context, _, id uuid.UUID) (*contact.Contact, error) {
	c, ok := m.contacts[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := c
	return &cp, nil
}

func (m *createTestContactLookup) ListByIDs(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]contact.Contact, error) {
	out := make([]contact.Contact, 0, len(ids))
	for _, id := range ids {
		if c, ok := m.contacts[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

type createTestTagLookup struct{}

func (createTestTagLookup) ListContactIDsByTag(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

type createTestProviderDir struct {
	byContact map[uuid.UUID][]provider.Profile
}

func (m *createTestProviderDir) ListByContact(_ context.Context, _, contactID uuid.UUID) ([]provider.Profile, error) {
	return m.byContact[contactID], nil
}

func (m *createTestProviderDir) ListByContactIDs(_ context.Context, _ uuid.UUID, contactIDs []uuid.UUID) (map[uuid.UUID][]provider.Profile, error) {
	out := make(map[uuid.UUID][]provider.Profile, len(contactIDs))
	for _, id := range contactIDs {
		if profiles, ok := m.byContact[id]; ok {
			out[id] = profiles
		}
	}
	return out, nil
}

func withCreateSnapshotting(contactID uuid.UUID) ServiceOption {
	profileID := uuid.New()
	contacts := &createTestContactLookup{contacts: map[uuid.UUID]contact.Contact{
		contactID: {
			ID:              contactID,
			CustomerName:    "Ada",
			ContactName:     "Ada",
			MarketingStatus: contact.MarketingSubscribed,
		},
	}}
	providers := &createTestProviderDir{byContact: map[uuid.UUID][]provider.Profile{
		contactID: {{
			ID:             profileID,
			ContactID:      contactID,
			Provider:       provider.WhatsApp,
			ProviderUserID: "628123456789",
			Phone:          "628123456789",
			Status:         provider.StatusActive,
		}},
	}}
	builder := recipient.NewRecipientSnapshotBuilder(recipient.NewWhatsAppRecipientResolver(providers))
	return WithRecipientSnapshotting(builder, contacts, createTestTagLookup{})
}

func TestService_Create_UsesSingleTransaction(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	channelID := uuid.New()
	templateID := uuid.New()
	contactID := uuid.New()

	repo := &spyCampaignRepo{}
	txMgr := &trackingTxManager{repo: repo}

	svc := NewService(
		repo,
		txMgr,
		&createTestTeamRepo{member: &team.Member{TeamID: teamID, UserID: userID}},
		&createTestChannelRepo{teamID: teamID, channelID: channelID},
		&createTestTemplateRepo{teamID: teamID, channelID: channelID, templateID: templateID},
		createTestSegmentRepo{},
		testCampaignRBAC(t),
		withCreateSnapshotting(contactID),
	)

	result, err := svc.Create(context.Background(), teamID, userID, "owner", validCreateInput(teamID, channelID, templateID, contactID))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result == nil || result.Campaign == nil || result.Job == nil || result.Snapshot == nil {
		t.Fatal("expected campaign, job, and snapshot in result")
	}
	if txMgr.runCount != 1 {
		t.Fatalf("tx.Run count = %d, want 1", txMgr.runCount)
	}
	if repo.createCalls != 1 || repo.jobCalls != 1 || repo.snapCalls != 1 || repo.recipientCalls != 1 {
		t.Fatalf("repo calls create=%d job=%d snap=%d recipients=%d, want 1 each",
			repo.createCalls, repo.jobCalls, repo.snapCalls, repo.recipientCalls)
	}
	if repo.softDeleteCalls != 0 {
		t.Fatalf("SoftDelete calls = %d, want 0", repo.softDeleteCalls)
	}
	if result.Snapshot.ResolvedCount != 1 {
		t.Fatalf("resolved_count = %d, want 1", result.Snapshot.ResolvedCount)
	}
}

func TestService_Create_SnapshotFailure_NoSoftDelete(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	channelID := uuid.New()
	templateID := uuid.New()
	contactID := uuid.New()

	repo := &spyCampaignRepo{snapErr: errors.New("snapshot insert failed")}
	txMgr := &trackingTxManager{repo: repo}

	svc := NewService(
		repo,
		txMgr,
		&createTestTeamRepo{member: &team.Member{TeamID: teamID, UserID: userID}},
		&createTestChannelRepo{teamID: teamID, channelID: channelID},
		&createTestTemplateRepo{teamID: teamID, channelID: channelID, templateID: templateID},
		createTestSegmentRepo{},
		testCampaignRBAC(t),
		withCreateSnapshotting(contactID),
	)

	_, err := svc.Create(context.Background(), teamID, userID, "owner", validCreateInput(teamID, channelID, templateID, contactID))
	if err == nil {
		t.Fatal("expected error from snapshot failure")
	}
	if repo.softDeleteCalls != 0 {
		t.Fatalf("SoftDelete calls = %d, want 0 (rollback handled by tx manager)", repo.softDeleteCalls)
	}
	if repo.createCalls != 1 || repo.jobCalls != 1 || repo.snapCalls != 1 {
		t.Fatalf("repo calls create=%d job=%d snap=%d, want 1 each", repo.createCalls, repo.jobCalls, repo.snapCalls)
	}
}

func TestService_Create_RejectsWhenNoWhatsAppProvider(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	channelID := uuid.New()
	templateID := uuid.New()
	contactID := uuid.New()

	repo := &spyCampaignRepo{}
	txMgr := &trackingTxManager{repo: repo}
	contacts := &createTestContactLookup{contacts: map[uuid.UUID]contact.Contact{
		contactID: {ID: contactID, MarketingStatus: contact.MarketingSubscribed},
	}}
	builder := recipient.NewRecipientSnapshotBuilder(recipient.NewWhatsAppRecipientResolver(&createTestProviderDir{}))

	svc := NewService(
		repo,
		txMgr,
		&createTestTeamRepo{member: &team.Member{TeamID: teamID, UserID: userID}},
		&createTestChannelRepo{teamID: teamID, channelID: channelID},
		&createTestTemplateRepo{teamID: teamID, channelID: channelID, templateID: templateID},
		createTestSegmentRepo{},
		testCampaignRBAC(t),
		WithRecipientSnapshotting(builder, contacts, createTestTagLookup{}),
	)

	_, err := svc.Create(context.Background(), teamID, userID, "owner", validCreateInput(teamID, channelID, templateID, contactID))
	if err == nil {
		t.Fatal("expected validation error when no WhatsApp provider")
	}
	if repo.createCalls != 0 {
		t.Fatalf("create should not run when snapshotting fails, got %d", repo.createCalls)
	}
}
