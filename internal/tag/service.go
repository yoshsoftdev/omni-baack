package tag

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

var (
	ErrForbidden    = errors.New("insufficient permissions")
	ErrNameRequired = errors.New("tag name is required")
	ErrInvalidColor = errors.New("invalid color format")
)

var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type Service struct {
	tags  Repository
	teams team.Repository
}

func NewService(tags Repository, teams team.Repository) *Service {
	return &Service{tags: tags, teams: teams}
}

type CreateInput struct {
	Name  string
	Color string
}

type UpdateInput struct {
	Name  string
	Color string
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID) ([]Tag, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.tags.ListByTeam(ctx, teamID)
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, role team.Role, input CreateInput) (*Tag, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	color := normalizeColor(input.Color)

	now := time.Now().UTC()
	tag := &Tag{
		ID:        uuid.New(),
		TeamID:    teamID,
		Name:      name,
		Color:     color,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.tags.Create(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, role team.Role, tagID uuid.UUID, input UpdateInput) (*Tag, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrNameRequired
	}

	tag, err := s.tags.FindByID(ctx, teamID, tagID)
	if err != nil {
		return nil, err
	}

	tag.Name = name
	tag.Color = normalizeColor(input.Color)
	tag.UpdatedAt = time.Now().UTC()

	if err := s.tags.Update(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, role team.Role, tagID uuid.UUID) error {
	if !canManage(role) {
		return ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return s.tags.SoftDelete(ctx, teamID, tagID)
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}

func normalizeColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return "#6366f1"
	}
	if !colorPattern.MatchString(color) {
		return "#6366f1"
	}
	return color
}
