package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type trackingSnapshotRepo struct {
	snap      *ReadinessSnapshot
	upsertCnt int
}

func (r *trackingSnapshotRepo) Upsert(ctx context.Context, snap *ReadinessSnapshot) error {
	r.upsertCnt++
	r.snap = snap
	return nil
}

func (r *trackingSnapshotRepo) FindByTeam(ctx context.Context, teamID uuid.UUID) (*ReadinessSnapshot, error) {
	if r.snap != nil && r.snap.TeamID == teamID {
		return r.snap, nil
	}
	return nil, errSnapshotNotFound
}

var errSnapshotNotFound = errNotFound("snapshot not found")

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

type workspaceStubTeamRepo struct{}

func (workspaceStubTeamRepo) Create(context.Context, *team.Team) error { return nil }
func (workspaceStubTeamRepo) FindByID(context.Context, uuid.UUID) (*team.Team, error) {
	return nil, team.ErrNotFound
}
func (workspaceStubTeamRepo) Update(context.Context, *team.Team) error { return nil }
func (workspaceStubTeamRepo) CreateMember(context.Context, *team.Member) error { return nil }
func (workspaceStubTeamRepo) FindPrimaryMembership(context.Context, uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (workspaceStubTeamRepo) FindMemberByUser(context.Context, uuid.UUID, uuid.UUID) (*team.Member, error) {
	return &team.Member{}, nil
}
func (workspaceStubTeamRepo) FindMemberByID(context.Context, uuid.UUID, uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (workspaceStubTeamRepo) ListMembers(context.Context, uuid.UUID) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (workspaceStubTeamRepo) ListMembersFiltered(context.Context, uuid.UUID, team.ListMembersFilter) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (workspaceStubTeamRepo) UpdateMemberRole(context.Context, uuid.UUID, uuid.UUID, team.Role, uuid.UUID) error {
	return nil
}
func (workspaceStubTeamRepo) UpdateMemberStatus(context.Context, uuid.UUID, uuid.UUID, team.MemberStatus) error {
	return nil
}
func (workspaceStubTeamRepo) SoftDeleteMember(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (workspaceStubTeamRepo) FindRoleIDByName(context.Context, string) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (workspaceStubTeamRepo) GetWorkspaceDetail(context.Context, uuid.UUID) (*team.WorkspaceDetail, error) {
	return nil, team.ErrNotFound
}
func (workspaceStubTeamRepo) CreateInvitation(context.Context, *team.Invitation) error { return nil }
func (workspaceStubTeamRepo) ListInvitations(context.Context, uuid.UUID) ([]team.Invitation, error) {
	return nil, nil
}

type workspaceStubRBACRepo struct{}

func (workspaceStubRBACRepo) LoadRolePermissions(context.Context) (map[string]map[string]bool, error) {
	return map[string]map[string]bool{
		string(team.RoleOwner): {rbac.PermUserView: true},
	}, nil
}
func (workspaceStubRBACRepo) ListRoles(context.Context) ([]rbac.Role, error) { return nil, nil }
func (workspaceStubRBACRepo) FindRoleByID(context.Context, uuid.UUID) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (workspaceStubRBACRepo) FindRoleByName(context.Context, string) (*rbac.Role, error) {
	return nil, rbac.ErrNotFound
}
func (workspaceStubRBACRepo) ListPermissions(context.Context) ([]rbac.Permission, error) { return nil, nil }
func (workspaceStubRBACRepo) ListPermissionsByRoleID(context.Context, uuid.UUID) ([]rbac.Permission, error) {
	return nil, nil
}
func (workspaceStubRBACRepo) GetPermissionMatrix(context.Context) ([]rbac.PermissionMatrixRow, error) {
	return nil, nil
}

func testWorkspaceService(snapshots SnapshotRepository) *Service {
	rbacSvc := rbac.NewService(workspaceStubRBACRepo{})
	_ = rbacSvc.WarmCache(context.Background())
	return &Service{
		teams:     workspaceStubTeamRepo{},
		rbac:      rbacSvc,
		snapshots: snapshots,
	}
}

func TestGetReadinessDoesNotWriteSnapshot(t *testing.T) {
	teamID := uuid.New()
	snapshots := &trackingSnapshotRepo{
		snap: &ReadinessSnapshot{
			TeamID:     teamID,
			Score:      75,
			Status:     "in_progress",
			Checks:     []ReadinessCheck{{Key: "channel_connected", Completed: true}},
			ComputedAt: time.Now().UTC(),
		},
	}

	svc := testWorkspaceService(snapshots)
	result, err := svc.GetReadiness(context.Background(), teamID, uuid.New(), string(team.RoleOwner))
	if err != nil {
		t.Fatalf("GetReadiness: %v", err)
	}
	if snapshots.upsertCnt != 0 {
		t.Fatalf("expected no snapshot writes, got %d", snapshots.upsertCnt)
	}
	if result.Score != 75 {
		t.Fatalf("expected score 75, got %d", result.Score)
	}
}

func TestGetOnboardingDoesNotWriteSnapshot(t *testing.T) {
	teamID := uuid.New()
	snapshots := &trackingSnapshotRepo{
		snap: &ReadinessSnapshot{
			TeamID: teamID,
			Score:  50,
			Onboarding: &OnboardingState{
				CurrentStep:    2,
				CompletedSteps: []int{1},
			},
			ComputedAt: time.Now().UTC(),
		},
	}

	svc := testWorkspaceService(snapshots)
	snapshot, err := svc.GetOnboarding(context.Background(), teamID, uuid.New(), string(team.RoleOwner))
	if err != nil {
		t.Fatalf("GetOnboarding: %v", err)
	}
	if snapshots.upsertCnt != 0 {
		t.Fatalf("expected no snapshot writes, got %d", snapshots.upsertCnt)
	}
	if snapshot.CurrentStep != 2 || snapshot.Score != 50 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
