package inbox

import (
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)
type ConversationCounter interface {
	CountOpenByAssignee(ctx context.Context, teamID, userID uuid.UUID) (int64, error)
}

// TeamLister lists team members for assignment.
type TeamLister interface {
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]team.MemberWithUser, error)
}

// AssignmentEngine assigns new conversations to team members.
type AssignmentEngine struct {
	teams         TeamLister
	settings      SettingsRepository
	conversations ConversationCounter
}

func NewAssignmentEngine(teams TeamLister, settings SettingsRepository, conversations ConversationCounter) *AssignmentEngine {
	return &AssignmentEngine{teams: teams, settings: settings, conversations: conversations}
}

// PickAssignee selects an agent for a new unassigned conversation.
func (e *AssignmentEngine) PickAssignee(ctx context.Context, teamID uuid.UUID) (*uuid.UUID, error) {
	settings, err := e.settings.Get(ctx, teamID)
	if err != nil {
		return nil, err
	}
	switch settings.AssignmentMode {
	case AssignmentRoundRobin:
		return e.roundRobin(ctx, teamID, settings)
	case AssignmentLeastLoaded:
		return e.leastLoaded(ctx, teamID)
	default:
		return nil, nil
	}
}

func (e *AssignmentEngine) roundRobin(ctx context.Context, teamID uuid.UUID, settings *TeamSettings) (*uuid.UUID, error) {
	members, err := e.teams.ListMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	var eligible []uuid.UUID
	for _, m := range members {
		if m.Status == team.MemberStatusActive {
			eligible = append(eligible, m.UserID)
		}
	}
	if len(eligible) == 0 {
		return nil, nil
	}
	idx := settings.RoundRobinCursor % len(eligible)
	next := settings.RoundRobinCursor + 1
	_ = e.settings.UpdateRoundRobinCursor(ctx, teamID, next)
	id := eligible[idx]
	return &id, nil
}

func (e *AssignmentEngine) leastLoaded(ctx context.Context, teamID uuid.UUID) (*uuid.UUID, error) {
	members, err := e.teams.ListMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	var best *uuid.UUID
	var bestCount int64 = -1
	for _, m := range members {
		if m.Status != team.MemberStatusActive {
			continue
		}
		count, err := e.conversations.CountOpenByAssignee(ctx, teamID, m.UserID)
		if err != nil {
			return nil, err
		}
		if best == nil || count < bestCount {
			id := m.UserID
			best = &id
			bestCount = count
		}
	}
	return best, nil
}
