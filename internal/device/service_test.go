package device

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type mockDeviceRepo struct {
	devices map[uuid.UUID]*Device
}

func (m *mockDeviceRepo) Create(ctx context.Context, device *Device) error {
	m.devices[device.ID] = device
	return nil
}

func (m *mockDeviceRepo) FindByID(ctx context.Context, teamID, deviceID uuid.UUID) (*Device, error) {
	d, ok := m.devices[deviceID]
	if !ok || d.TeamID != teamID || d.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return d, nil
}

func (m *mockDeviceRepo) FindByPhoneNumberID(ctx context.Context, phoneNumberID string) (*Device, error) {
	for _, d := range m.devices {
		if d.PhoneNumberID != nil && *d.PhoneNumberID == phoneNumberID && d.DeletedAt == nil {
			return d, nil
		}
	}
	return nil, ErrNotFound
}

func (m *mockDeviceRepo) FindBySlug(ctx context.Context, slug string) (*Device, error) {
	for _, d := range m.devices {
		if d.Slug != nil && *d.Slug == slug && d.DeletedAt == nil {
			return d, nil
		}
	}
	return nil, ErrNotFound
}

func (m *mockDeviceRepo) ListByTeam(ctx context.Context, teamID uuid.UUID) ([]Device, error) {
	var list []Device
	for _, d := range m.devices {
		if d.TeamID == teamID && d.DeletedAt == nil {
			list = append(list, *d)
		}
	}
	return list, nil
}

func (m *mockDeviceRepo) Update(ctx context.Context, device *Device) error {
	m.devices[device.ID] = device
	return nil
}

func (m *mockDeviceRepo) SoftDelete(ctx context.Context, teamID, deviceID uuid.UUID) error {
	d, err := m.FindByID(ctx, teamID, deviceID)
	if err != nil {
		return err
	}
	now := time.Now()
	d.DeletedAt = &now
	m.devices[deviceID] = d
	return nil
}

type mockTeamRepo struct {
	members map[string]team.Member
}

func (m *mockTeamRepo) Create(ctx context.Context, t *team.Team) error { return nil }
func (m *mockTeamRepo) FindByID(ctx context.Context, id uuid.UUID) (*team.Team, error) {
	return nil, team.ErrNotFound
}
func (m *mockTeamRepo) Update(ctx context.Context, t *team.Team) error { return nil }
func (m *mockTeamRepo) CreateMember(ctx context.Context, member *team.Member) error { return nil }
func (m *mockTeamRepo) FindPrimaryMembership(ctx context.Context, userID uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (m *mockTeamRepo) FindMemberByUser(ctx context.Context, teamID, userID uuid.UUID) (*team.Member, error) {
	key := teamID.String() + ":" + userID.String()
	member, ok := m.members[key]
	if !ok {
		return nil, team.ErrMemberNotFound
	}
	return &member, nil
}
func (m *mockTeamRepo) FindMemberByID(ctx context.Context, teamID, memberID uuid.UUID) (*team.Member, error) {
	return nil, team.ErrMemberNotFound
}
func (m *mockTeamRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]team.MemberWithUser, error) {
	return nil, nil
}
func (m *mockTeamRepo) UpdateMemberRole(ctx context.Context, teamID, memberID uuid.UUID, role team.Role) error {
	return nil
}
func (m *mockTeamRepo) SoftDeleteMember(ctx context.Context, teamID, memberID uuid.UUID) error {
	return nil
}

func TestService_Create(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	deviceRepo := &mockDeviceRepo{devices: map[uuid.UUID]*Device{}}
	teamRepo := &mockTeamRepo{
		members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {
				ID: uuid.New(), TeamID: teamID, UserID: userID, Role: team.RoleOwner,
			},
		},
	}
	svc := NewService(deviceRepo, teamRepo, metaclient.NewClient(""))

	created, err := svc.Create(context.Background(), teamID, userID, team.RoleOwner, CreateInput{Name: "Sales WA"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusDisconnected || created.Provider != "meta" {
		t.Fatalf("unexpected device: %+v", created)
	}
}

func TestService_Connect_MetaRequired(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	deviceRepo := &mockDeviceRepo{devices: map[uuid.UUID]*Device{}}
	teamRepo := &mockTeamRepo{
		members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {
				ID: uuid.New(), TeamID: teamID, UserID: userID, Role: team.RoleOwner,
			},
		},
	}
	svc := NewService(deviceRepo, teamRepo, metaclient.NewClient(""))

	created, err := svc.Create(context.Background(), teamID, userID, team.RoleOwner, CreateInput{Name: "Sales WA"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = svc.Connect(context.Background(), teamID, userID, team.RoleOwner, created.ID, ConnectInput{})
	if err != ErrMetaRequired {
		t.Fatalf("Connect() error = %v, want ErrMetaRequired", err)
	}
}

func TestService_Create_ForbiddenForAgent(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()
	svc := NewService(&mockDeviceRepo{devices: map[uuid.UUID]*Device{}}, &mockTeamRepo{
		members: map[string]team.Member{
			teamID.String() + ":" + userID.String(): {
				ID: uuid.New(), TeamID: teamID, UserID: userID, Role: team.RoleAgent,
			},
		},
	}, metaclient.NewClient(""))

	_, err := svc.Create(context.Background(), teamID, userID, team.RoleAgent, CreateInput{Name: "WA"})
	if err != ErrForbidden {
		t.Fatalf("Create() error = %v", err)
	}
}
