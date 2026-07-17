package provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service owns provider identity, linking, unlinking, and source-of-truth policy.
type Service struct {
	profiles Repository
	teams    team.Repository
	rbac     *rbac.Service
}

func NewService(profiles Repository, teams team.Repository, rbacService *rbac.Service) *Service {
	return &Service{profiles: profiles, teams: teams, rbac: rbacService}
}

// Create is used by canonical provider ingestion after adapter normalization.
func (s *Service) Create(ctx context.Context, profile *Profile) (*Profile, error) {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	if profile.ConnectedAt.IsZero() {
		profile.ConnectedAt = now
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	if profile.Status == "" {
		profile.Status = StatusActive
	}
	profile.ProviderUserID = strings.TrimSpace(profile.ProviderUserID)
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	if err := serviceerrors.MapRepository(s.profiles.Create(ctx, profile)); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, role string) ([]Profile, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactView); err != nil {
		return nil, err
	}
	return s.profiles.List(ctx, teamID)
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, role string, profileID uuid.UUID) (*Profile, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactView); err != nil {
		return nil, err
	}
	profile, err := s.profiles.FindByID(ctx, teamID, profileID)
	return profile, serviceerrors.MapRepository(err)
}

func (s *Service) ListByContact(ctx context.Context, teamID, userID uuid.UUID, role string, contactID uuid.UUID) ([]Profile, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactView); err != nil {
		return nil, err
	}
	return s.profiles.ListByContact(ctx, teamID, contactID)
}

func (s *Service) Link(ctx context.Context, teamID, userID uuid.UUID, role string, profileID, targetContactID uuid.UUID, reason string) (*LinkResult, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	profile, err := s.profiles.FindByID(ctx, teamID, profileID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if profile.ContactID == targetContactID {
		return nil, ErrCannotLinkToSameContact
	}
	result, err := s.profiles.Link(ctx, teamID, profileID, targetContactID, userID, reason)
	return result, serviceerrors.MapRepository(err)
}

func (s *Service) Unlink(ctx context.Context, teamID, userID uuid.UUID, role string, profileID uuid.UUID, reason string) (*UnlinkResult, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	result, err := s.profiles.Unlink(ctx, teamID, profileID, userID, reason)
	return result, serviceerrors.MapRepository(err)
}

func (s *Service) SetSourceOfTruth(ctx context.Context, teamID, userID uuid.UUID, role string, profileID uuid.UUID) (*Profile, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	profile, err := s.profiles.FindByID(ctx, teamID, profileID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if profile.Provider != WhatsApp {
		return nil, ErrSourceOfTruthMustBeWhatsApp
	}
	result, err := s.profiles.SetSourceOfTruth(ctx, teamID, profileID, userID)
	return result, serviceerrors.MapRepository(err)
}

func (s *Service) ListActivities(ctx context.Context, teamID, userID uuid.UUID, role string, contactID uuid.UUID, limit int) ([]Activity, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermContactView); err != nil {
		return nil, err
	}
	return s.profiles.ListActivities(ctx, teamID, contactID, limit)
}

func (s *Service) authorize(ctx context.Context, teamID, userID uuid.UUID, role, permission string) error {
	if s.rbac != nil && !s.rbac.Require(role, permission) {
		return serviceerrors.ErrForbidden
	}
	if s.teams == nil {
		return nil
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		if errors.Is(err, team.ErrMemberNotFound) || errors.Is(err, repository.ErrNotFound) {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}
