package segment

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Service (SegmentService) manages contact segments and rules.
type Service struct {
	segments Repository
	teams    team.Repository
	rbac     *rbac.Service
}

func NewService(segments Repository, teams team.Repository, rbacSvc *rbac.Service) *Service {
	return &Service{segments: segments, teams: teams, rbac: rbacSvc}
}

type CreateInput struct {
	Name        string
	Description string
	Color       string
	IsDynamic   bool
	Rules       []RuleInput
}

type RuleInput struct {
	Field     RuleField
	Operator  RuleOperator
	Value     string
	SortOrder int
}

type UpdateInput struct {
	Name        *string
	Description *string
	Color       *string
	IsDynamic   *bool
	Rules       []RuleInput
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

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Segment, []Rule, error) {
	if err := s.authorize(actorRole, rbac.PermContactCreate); err != nil {
		return nil, nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, nil, serviceerrors.NewValidation("name", "is required")
	}
	rules, err := buildRules(input.Rules)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	seg := &Segment{
		ID:          uuid.New(),
		TeamID:      teamID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Color:       defaultColor(input.Color),
		IsDynamic:   input.IsDynamic,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for i := range rules {
		rules[i].SegmentID = seg.ID
		rules[i].CreatedAt = now
	}
	if err := serviceerrors.MapRepository(s.segments.Create(ctx, seg, rules)); err != nil {
		return nil, nil, err
	}
	return seg, rules, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, segmentID uuid.UUID) (*Segment, []Rule, error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return nil, nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, nil, err
	}
	seg, rules, err := s.segments.FindByID(ctx, teamID, segmentID)
	if err != nil {
		return nil, nil, serviceerrors.MapRepository(err)
	}
	return seg, rules, nil
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Segment], error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return repository.Page[Segment]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Segment]{}, err
	}
	result, err := s.segments.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Segment]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, segmentID uuid.UUID, input UpdateInput) (*Segment, []Rule, error) {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return nil, nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, nil, err
	}

	seg, _, err := s.segments.FindByID(ctx, teamID, segmentID)
	if err != nil {
		return nil, nil, serviceerrors.MapRepository(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, nil, serviceerrors.NewValidation("name", "is required")
		}
		seg.Name = name
	}
	if input.Description != nil {
		seg.Description = strings.TrimSpace(*input.Description)
	}
	if input.Color != nil {
		seg.Color = defaultColor(*input.Color)
	}
	if input.IsDynamic != nil {
		seg.IsDynamic = *input.IsDynamic
	}
	seg.UpdatedAt = time.Now().UTC()

	var rules []Rule
	if input.Rules != nil {
		rules, err = buildRules(input.Rules)
		if err != nil {
			return nil, nil, err
		}
		for i := range rules {
			rules[i].SegmentID = seg.ID
			rules[i].CreatedAt = time.Now().UTC()
		}
	}
	if err := serviceerrors.MapRepository(s.segments.Update(ctx, seg, rules)); err != nil {
		return nil, nil, err
	}
	return s.segments.FindByID(ctx, teamID, segmentID)
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, segmentID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermContactDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.segments.SoftDelete(ctx, teamID, segmentID))
}

// ValidateExists ensures the segment belongs to the team (used by CampaignService).
func (s *Service) ValidateExists(ctx context.Context, teamID, segmentID uuid.UUID) error {
	_, _, err := s.segments.FindByID(ctx, teamID, segmentID)
	return serviceerrors.MapRepository(err)
}

func buildRules(inputs []RuleInput) ([]Rule, error) {
	if len(inputs) == 0 {
		return nil, serviceerrors.NewValidation("rules", "at least one rule is required")
	}
	rules := make([]Rule, 0, len(inputs))
	for i, in := range inputs {
		if err := validateRule(in); err != nil {
			return nil, err
		}
		sortOrder := in.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		rules = append(rules, Rule{
			ID:        uuid.New(),
			Field:     in.Field,
			Operator:  in.Operator,
			Value:     strings.TrimSpace(in.Value),
			SortOrder: sortOrder,
		})
	}
	return rules, nil
}

func validateRule(in RuleInput) error {
	switch in.Field {
	case RuleFieldTag, RuleFieldCity, RuleFieldLastActivity, RuleFieldMarketingStatus, RuleFieldSource:
	default:
		return serviceerrors.NewValidation("rules.field", "invalid field")
	}
	switch in.Operator {
	case OpEquals, OpNotEquals, OpContains, OpLessThan, OpGreaterThan:
	default:
		return serviceerrors.NewValidation("rules.operator", "invalid operator")
	}
	if strings.TrimSpace(in.Value) == "" {
		return serviceerrors.NewValidation("rules.value", "is required")
	}
	return nil
}

func defaultColor(color string) string {
	c := strings.TrimSpace(color)
	if c == "" {
		return "#6366f1"
	}
	return c
}
