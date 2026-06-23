package quota

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/planlimits"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Counter repositories used for quota checks (avoids import cycles).
type ContactCounter interface {
	CountByTeam(ctx context.Context, teamID uuid.UUID) (int64, error)
}

type ChannelCounter interface {
	CountByTeam(ctx context.Context, teamID uuid.UUID) (int64, error)
}

// Service enforces workspace plan quotas.
type Service struct {
	teams    team.Repository
	channels ChannelCounter
	contacts ContactCounter
}

func NewService(teams team.Repository, channels ChannelCounter, contacts ContactCounter) *Service {
	return &Service{teams: teams, channels: channels, contacts: contacts}
}

type Snapshot struct {
	ContactLimit int
	ContactCount int64
	ChannelLimit int
	ChannelCount int64
	MemberLimit  int
	MemberCount  int
}

func (s *Service) Snapshot(ctx context.Context, teamID uuid.UUID) (*Snapshot, error) {
	t, err := s.teams.FindByID(ctx, teamID)
	if err != nil {
		if errors.Is(err, team.ErrNotFound) {
			return nil, serviceerrors.ErrNotFound
		}
		return nil, err
	}
	contactCount, err := s.contacts.CountByTeam(ctx, teamID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	channelCount, err := s.channels.CountByTeam(ctx, teamID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	members, err := s.teams.ListMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		ContactLimit: t.ContactLimit,
		ContactCount: contactCount,
		ChannelLimit: t.ChannelLimit,
		ChannelCount: channelCount,
		MemberLimit:  planlimits.MemberLimit(string(t.PlanType)),
		MemberCount:  len(members),
	}, nil
}

func (s *Service) AssertContactHeadroom(ctx context.Context, teamID uuid.UUID, additional int) error {
	if additional <= 0 {
		return nil
	}
	snap, err := s.Snapshot(ctx, teamID)
	if err != nil {
		return err
	}
	if snap.ContactCount+int64(additional) > int64(snap.ContactLimit) {
		return serviceerrors.ErrQuotaExceeded
	}
	return nil
}

func (s *Service) AssertChannelHeadroom(ctx context.Context, teamID uuid.UUID) error {
	snap, err := s.Snapshot(ctx, teamID)
	if err != nil {
		return err
	}
	if int(snap.ChannelCount) >= snap.ChannelLimit {
		return serviceerrors.ErrQuotaExceeded
	}
	return nil
}

func (s *Service) AssertMemberHeadroom(ctx context.Context, teamID uuid.UUID) error {
	snap, err := s.Snapshot(ctx, teamID)
	if err != nil {
		return err
	}
	if snap.MemberLimit == 0 {
		return nil
	}
	if snap.MemberCount >= snap.MemberLimit {
		return serviceerrors.ErrQuotaExceeded
	}
	return nil
}
