package inbox_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type memSettings struct {
	settings *inbox.TeamSettings
}

func (m *memSettings) Get(_ context.Context, teamID uuid.UUID) (*inbox.TeamSettings, error) {
	if m.settings == nil {
		return &inbox.TeamSettings{TeamID: teamID, AssignmentMode: inbox.AssignmentManual}, nil
	}
	return m.settings, nil
}
func (m *memSettings) UpdateRoundRobinCursor(_ context.Context, _ uuid.UUID, cursor int) error {
	if m.settings != nil {
		m.settings.RoundRobinCursor = cursor
	}
	return nil
}

type memTeams struct {
	members []team.MemberWithUser
}

func (m *memTeams) ListMembers(_ context.Context, _ uuid.UUID) ([]team.MemberWithUser, error) {
	return m.members, nil
}

type memCounter struct {
	counts map[uuid.UUID]int64
}

func (m *memCounter) CountOpenByAssignee(_ context.Context, _, userID uuid.UUID) (int64, error) {
	return m.counts[userID], nil
}

func TestAssignmentRoundRobin(t *testing.T) {
	teamID := uuid.New()
	u1, u2 := uuid.New(), uuid.New()
	settings := &inbox.TeamSettings{TeamID: teamID, AssignmentMode: inbox.AssignmentRoundRobin}
	engine := inbox.NewAssignmentEngine(
		&memTeams{members: []team.MemberWithUser{
			{UserID: u1, Status: team.MemberStatusActive},
			{UserID: u2, Status: team.MemberStatusActive},
		}},
		&memSettings{settings: settings},
		&memCounter{counts: map[uuid.UUID]int64{}},
	)
	a1, err := engine.PickAssignee(context.Background(), teamID)
	if err != nil || a1 == nil || *a1 != u1 {
		t.Fatalf("first assignee: %v %v", a1, err)
	}
	a2, err := engine.PickAssignee(context.Background(), teamID)
	if err != nil || a2 == nil || *a2 != u2 {
		t.Fatalf("second assignee: %v %v", a2, err)
	}
}

func TestAssignmentLeastLoaded(t *testing.T) {
	teamID := uuid.New()
	u1, u2 := uuid.New(), uuid.New()
	engine := inbox.NewAssignmentEngine(
		&memTeams{members: []team.MemberWithUser{
			{UserID: u1, Status: team.MemberStatusActive},
			{UserID: u2, Status: team.MemberStatusActive},
		}},
		&memSettings{settings: &inbox.TeamSettings{TeamID: teamID, AssignmentMode: inbox.AssignmentLeastLoaded}},
		&memCounter{counts: map[uuid.UUID]int64{u1: 5, u2: 1}},
	)
	got, err := engine.PickAssignee(context.Background(), teamID)
	if err != nil || got == nil || *got != u2 {
		t.Fatalf("least loaded: %v %v", got, err)
	}
}

var _ = conversation.StatusOpen
