package campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
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

type createTestChannelRepo struct {
	channelID uuid.UUID
	teamID    uuid.UUID
}

func (m *createTestChannelRepo) FindByID(_ context.Context, teamID, id uuid.UUID) (*channel.Channel, error) {
	if teamID != m.teamID || id != m.channelID {
		return nil, repository.ErrNotFound
	}
	return &channel.Channel{ID: id, TeamID: teamID, Status: channel.StatusConnected}, nil
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

func (m *createTestTemplateRepo) Create(context.Context, *template.Template) error { return nil }
func (m *createTestTemplateRepo) List(context.Context, uuid.UUID, template.ListFilter, repository.PageParams) (repository.Page[template.Template], error) {
	return repository.Page[template.Template]{}, nil
}
func (m *createTestTemplateRepo) UpsertFromSync(context.Context, *template.Template) error { return nil }
func (m *createTestTemplateRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

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

func testCampaignRBAC(t *testing.T) *rbac.Service {
	t.Helper()
	svc := rbac.NewService(createTestRbacRepo{})
	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("WarmCache: %v", err)
	}
	return svc
}

func validCreateInput(teamID, channelID, templateID uuid.UUID) CreateInput {
	return CreateInput{
		Name:       "Summer Promo",
		ChannelID:  channelID,
		TemplateID: templateID,
		Audiences: []AudienceInput{
			{SourceType: AudienceTag, SourceID: uuid.New()},
		},
		SendMode: SendNow,
	}
}

func TestService_Create_UsesSingleTransaction(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	channelID := uuid.New()
	templateID := uuid.New()

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
	)

	result, err := svc.Create(context.Background(), teamID, userID, "owner", validCreateInput(teamID, channelID, templateID))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result == nil || result.Campaign == nil || result.Job == nil || result.Snapshot == nil {
		t.Fatal("expected campaign, job, and snapshot in result")
	}
	if txMgr.runCount != 1 {
		t.Fatalf("tx.Run count = %d, want 1", txMgr.runCount)
	}
	if repo.createCalls != 1 || repo.jobCalls != 1 || repo.snapCalls != 1 {
		t.Fatalf("repo calls create=%d job=%d snap=%d, want 1 each", repo.createCalls, repo.jobCalls, repo.snapCalls)
	}
	if repo.softDeleteCalls != 0 {
		t.Fatalf("SoftDelete calls = %d, want 0", repo.softDeleteCalls)
	}
}

func TestService_Create_SnapshotFailure_NoSoftDelete(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	channelID := uuid.New()
	templateID := uuid.New()

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
	)

	_, err := svc.Create(context.Background(), teamID, userID, "owner", validCreateInput(teamID, channelID, templateID))
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
