package report

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type mockReportRepo struct {
	funnel      *CampaignFunnel
	clicksTeam  uuid.UUID
	clicksCalls int
}

func (m *mockReportRepo) AggregateDashboard(ctx context.Context, teamID uuid.UUID, range_ DateRange, channelID *uuid.UUID) (*DashboardStats, error) {
	return nil, nil
}

func (m *mockReportRepo) ListDailyStats(ctx context.Context, teamID uuid.UUID, range_ DateRange) ([]DailyStat, error) {
	return nil, nil
}

func (m *mockReportRepo) IncrementDailyStat(ctx context.Context, teamID uuid.UUID, statDate time.Time, delta DailyStatDelta) error {
	return nil
}

func (m *mockReportRepo) GetCampaignFunnel(ctx context.Context, teamID, campaignID uuid.UUID) (*CampaignFunnel, error) {
	if m.funnel == nil || m.funnel.TeamID != teamID {
		return nil, repository.ErrNotFound
	}
	return m.funnel, nil
}

func (m *mockReportRepo) ListCampaignFunnels(ctx context.Context, teamID uuid.UUID, range_ DateRange, page repository.PageParams) (repository.Page[CampaignFunnel], error) {
	return repository.Page[CampaignFunnel]{}, nil
}

func (m *mockReportRepo) RecordLinkClick(ctx context.Context, click *LinkClick) error {
	return nil
}

func (m *mockReportRepo) ListLinkClicks(ctx context.Context, teamID, campaignID uuid.UUID, page repository.PageParams) (repository.Page[LinkClick], error) {
	m.clicksCalls++
	m.clicksTeam = teamID
	return repository.Page[LinkClick]{Items: []LinkClick{{TeamID: teamID, CampaignID: campaignID}}}, nil
}

type stubTeamRepo struct{}

func (stubTeamRepo) Create(context.Context, *team.Team) error { return nil }
func (stubTeamRepo) FindByID(context.Context, uuid.UUID) (*team.Team, error) {
	return nil, team.ErrNotFound
}
func (stubTeamRepo) Update(context.Context, *team.Team) error { return nil }
func (stubTeamRepo) CreateMember(context.Context, *team.Member) error { return nil }
func (stubTeamRepo) FindPrimaryMembership(context.Context, uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (stubTeamRepo) FindMemberByUser(context.Context, uuid.UUID, uuid.UUID) (*team.Member, error) {
	return &team.Member{}, nil
}
func (stubTeamRepo) FindMemberByID(context.Context, uuid.UUID, uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (stubTeamRepo) ListMembers(context.Context, uuid.UUID) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (stubTeamRepo) ListMembersFiltered(context.Context, uuid.UUID, team.ListMembersFilter) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (stubTeamRepo) UpdateMemberRole(context.Context, uuid.UUID, uuid.UUID, team.Role, uuid.UUID) error {
	return nil
}
func (stubTeamRepo) UpdateMemberStatus(context.Context, uuid.UUID, uuid.UUID, team.MemberStatus) error {
	return nil
}
func (stubTeamRepo) SoftDeleteMember(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (stubTeamRepo) FindRoleIDByName(context.Context, string) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (stubTeamRepo) GetWorkspaceDetail(context.Context, uuid.UUID) (*team.WorkspaceDetail, error) {
	return nil, team.ErrNotFound
}
func (stubTeamRepo) CreateInvitation(context.Context, *team.Invitation) error { return nil }
func (stubTeamRepo) ListInvitations(context.Context, uuid.UUID) ([]team.Invitation, error) {
	return nil, nil
}

type stubRBACRepo struct{}

func (stubRBACRepo) LoadRolePermissions(context.Context) (map[string]map[string]bool, error) {
	return map[string]map[string]bool{
		string(team.RoleOwner): {rbac.PermReportView: true},
	}, nil
}
func (stubRBACRepo) ListRoles(context.Context) ([]rbac.Role, error) { return nil, nil }
func (stubRBACRepo) FindRoleByID(context.Context, uuid.UUID) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (stubRBACRepo) FindRoleByName(context.Context, string) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (stubRBACRepo) ListPermissions(context.Context) ([]rbac.Permission, error) { return nil, nil }
func (stubRBACRepo) ListPermissionsByRoleID(context.Context, uuid.UUID) ([]rbac.Permission, error) {
	return nil, nil
}
func (stubRBACRepo) GetPermissionMatrix(context.Context) ([]rbac.PermissionMatrixRow, error) {
	return nil, nil
}

func testReportService(reports Repository) *Service {
	rbacSvc := rbac.NewService(stubRBACRepo{})
	_ = rbacSvc.WarmCache(context.Background())
	return NewService(reports, stubTeamRepo{}, rbacSvc)
}

func TestListLinkClicksRequiresCampaignOwnership(t *testing.T) {
	teamA := uuid.New()
	teamB := uuid.New()
	campaignID := uuid.New()
	userID := uuid.New()

	reports := &mockReportRepo{
		funnel: &CampaignFunnel{CampaignID: campaignID, TeamID: teamA},
	}
	svc := testReportService(reports)

	_, err := svc.ListLinkClicks(context.Background(), teamB, userID, string(team.RoleOwner), campaignID, repository.PageParams{Page: 1, PageSize: 20})
	if err == nil {
		t.Fatal("expected error when campaign belongs to another team")
	}
	if !serviceerrors.IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
	if reports.clicksCalls != 0 {
		t.Fatalf("expected zero click list calls, got %d", reports.clicksCalls)
	}
}

func TestListLinkClicksPassesTeamScope(t *testing.T) {
	teamID := uuid.New()
	campaignID := uuid.New()
	userID := uuid.New()

	reports := &mockReportRepo{
		funnel: &CampaignFunnel{CampaignID: campaignID, TeamID: teamID},
	}
	svc := testReportService(reports)

	page, err := svc.ListLinkClicks(context.Background(), teamID, userID, string(team.RoleOwner), campaignID, repository.PageParams{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListLinkClicks: %v", err)
	}
	if reports.clicksCalls != 1 {
		t.Fatalf("expected one click list call, got %d", reports.clicksCalls)
	}
	if reports.clicksTeam != teamID {
		t.Fatalf("expected team %s, got %s", teamID, reports.clicksTeam)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(page.Items))
	}
}
