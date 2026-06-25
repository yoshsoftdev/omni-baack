package operations

import (
	"context"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service exposes read-only operational visibility.
type Service struct {
	repo    Repository
	workers WorkerCounter
	teams   team.Repository
	rbac    *rbac.Service
}

func NewService(repo Repository, workers WorkerCounter, teams team.Repository, rbacSvc *rbac.Service) *Service {
	return &Service{repo: repo, workers: workers, teams: teams, rbac: rbacSvc}
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

// GetBroadcastStatus returns pipeline queue and recovery metrics.
func (s *Service) GetBroadcastStatus(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*BroadcastStatus, error) {
	if err := s.authorize(actorRole, rbac.PermWorkspaceManage); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.repo.GetBroadcastStatus(ctx, 0, 0)
}

// GetWorkerStatus returns registered worker counts from Redis heartbeats.
func (s *Service) GetWorkerStatus(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*WorkerStatus, error) {
	if err := s.authorize(actorRole, rbac.PermWorkspaceManage); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.workers == nil {
		return &WorkerStatus{}, nil
	}
	orch, err := s.workers.CountOrchestrators(ctx)
	if err != nil {
		return nil, err
	}
	send, err := s.workers.CountSendWorkers(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.workers.CountEventConsumers(ctx)
	if err != nil {
		return nil, err
	}
	return &WorkerStatus{
		Orchestrators:  orch,
		SendWorkers:    send,
		EventConsumers: events,
	}, nil
}
