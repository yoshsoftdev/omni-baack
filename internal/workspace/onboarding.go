package workspace

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const maxOnboardingStep = 6

type OnboardingSnapshot struct {
	CurrentStep    int   `json:"current_step"`
	CompletedSteps []int `json:"completed_steps"`
	Score          int   `json:"score"`
}

func (s *Service) GetOnboarding(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*OnboardingSnapshot, error) {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	state, score, err := s.loadOnboardingState(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &OnboardingSnapshot{
		CurrentStep:    state.CurrentStep,
		CompletedSteps: state.CompletedSteps,
		Score:          score,
	}, nil
}

func (s *Service) CompleteOnboardingStep(ctx context.Context, teamID, userID uuid.UUID, actorRole string, step int) error {
	if err := s.authorize(actorRole, rbac.PermUserView); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	if step < 1 || step > maxOnboardingStep {
		return serviceerrors.NewValidation("step", "must be between 1 and 6")
	}

	state, _, err := s.loadOnboardingState(ctx, teamID)
	if err != nil {
		return err
	}
	if !containsStep(state.CompletedSteps, step) {
		state.CompletedSteps = append(state.CompletedSteps, step)
		sort.Ints(state.CompletedSteps)
	}
	if step >= state.CurrentStep {
		state.CurrentStep = step + 1
		if state.CurrentStep > maxOnboardingStep {
			state.CurrentStep = maxOnboardingStep
		}
	}

	readiness, err := computeReadiness(ctx, s.readinessDeps(), teamID)
	if err != nil {
		return err
	}
	if s.snapshots == nil {
		return nil
	}
	return serviceerrors.MapRepository(s.snapshots.Upsert(ctx, &ReadinessSnapshot{
		TeamID:         teamID,
		Score:          readiness.Score,
		CompletedCount: countCompleted(readiness.Checks),
		TotalCount:     totalReadinessChecks,
		Status:         readiness.Status,
		Checks:         readiness.Checks,
		Onboarding:     state,
		ComputedAt:     time.Now().UTC(),
	}))
}

func (s *Service) loadOnboardingState(ctx context.Context, teamID uuid.UUID) (*OnboardingState, int, error) {
	readiness, err := computeReadiness(ctx, s.readinessDeps(), teamID)
	if err != nil {
		return nil, 0, err
	}
	state := &OnboardingState{CurrentStep: 1, CompletedSteps: []int{}}
	if s.snapshots != nil {
		if existing, err := s.snapshots.FindByTeam(ctx, teamID); err == nil && existing.Onboarding != nil {
			state = existing.Onboarding
		}
	}
	if len(state.CompletedSteps) == 0 {
		state.CompletedSteps = []int{}
	}
	return state, readiness.Score, nil
}

func containsStep(steps []int, step int) bool {
	for _, value := range steps {
		if value == step {
			return true
		}
	}
	return false
}

func countCompleted(checks []ReadinessCheck) int {
	count := 0
	for _, check := range checks {
		if check.Completed {
			count++
		}
	}
	return count
}
