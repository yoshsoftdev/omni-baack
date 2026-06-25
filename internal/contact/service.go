package contact

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

var phonePattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// Service (ContactService) manages workspace contacts.
type Service struct {
	contacts  Repository
	teams     team.Repository
	quota     *quota.Service
	rbac      *rbac.Service
}

func NewService(
	contacts Repository,
	teams team.Repository,
	quotaSvc *quota.Service,
	rbacSvc *rbac.Service,
) *Service {
	return &Service{
		contacts: contacts,
		teams:    teams,
		quota:    quotaSvc,
		rbac:     rbacSvc,
	}
}

type CreateInput struct {
	Name            string
	Phone           string
	Email           *string
	Notes           *string
	City            *string
	Language        *string
	CustomerType    *string
	Source          Source
	MarketingStatus MarketingStatus
}

type UpdateInput struct {
	Name            *string
	Phone           *string
	Email           *string
	Notes           *string
	City            *string
	Language        *string
	CustomerType    *string
	MarketingStatus *MarketingStatus
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

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateInput) (*Contact, error) {
	if err := s.authorize(actorRole, rbac.PermContactCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if err := s.quota.AssertContactHeadroom(ctx, teamID, 1); err != nil {
		return nil, err
	}

	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return nil, err
	}
	if err := validateMarketingStatus(input.MarketingStatus); err != nil {
		return nil, err
	}
	if _, err := s.contacts.FindByPhone(ctx, teamID, phone); err == nil {
		return nil, serviceerrors.ErrConflict
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, serviceerrors.MapRepository(err)
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, serviceerrors.NewValidation("name", "is required")
	}

	source := input.Source
	if source == "" {
		source = SourceManual
	}

	now := time.Now().UTC()
	c := &Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		Name:            name,
		Phone:           phone,
		Email:           input.Email,
		Notes:           input.Notes,
		City:            input.City,
		Language:        input.Language,
		CustomerType:    input.CustomerType,
		Source:          source,
		MarketingStatus: input.MarketingStatus,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if c.MarketingStatus == "" {
		c.MarketingStatus = MarketingUnknown
	}
	if err := serviceerrors.MapRepository(s.contacts.Create(ctx, c)); err != nil {
		return nil, err
	}
	return s.enrichContact(ctx, teamID, c)
}

func (s *Service) Get(ctx context.Context, teamID, userID uuid.UUID, actorRole string, contactID uuid.UUID) (*Contact, error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	c, err := s.contacts.FindByID(ctx, teamID, contactID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return s.enrichContact(ctx, teamID, c)
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, actorRole string, filter ListFilter, page repository.PageParams) (repository.Page[Contact], error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return repository.Page[Contact]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Contact]{}, err
	}
	result, err := s.contacts.List(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Contact]{}, serviceerrors.MapRepository(err)
	}
	if err := s.enrichContacts(ctx, teamID, result.Items); err != nil {
		return repository.Page[Contact]{}, err
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, actorRole string, contactID uuid.UUID, input UpdateInput) (*Contact, error) {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	c, err := s.contacts.FindByID(ctx, teamID, contactID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		c.Name = name
	}
	if input.Phone != nil {
		phone, err := normalizePhone(*input.Phone)
		if err != nil {
			return nil, err
		}
		if phone != c.Phone {
			if existing, err := s.contacts.FindByPhone(ctx, teamID, phone); err == nil && existing.ID != c.ID {
				return nil, serviceerrors.ErrConflict
			} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return nil, serviceerrors.MapRepository(err)
			}
			c.Phone = phone
		}
	}
	if input.Email != nil {
		c.Email = input.Email
	}
	if input.Notes != nil {
		c.Notes = input.Notes
	}
	if input.City != nil {
		c.City = input.City
	}
	if input.Language != nil {
		c.Language = input.Language
	}
	if input.CustomerType != nil {
		c.CustomerType = input.CustomerType
	}
	if input.MarketingStatus != nil {
		if err := validateMarketingStatus(*input.MarketingStatus); err != nil {
			return nil, err
		}
		c.MarketingStatus = *input.MarketingStatus
	}
	c.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.contacts.Update(ctx, c)); err != nil {
		return nil, err
	}
	return s.enrichContact(ctx, teamID, c)
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, contactID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermContactDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.contacts.SoftDelete(ctx, teamID, contactID))
}

func normalizePhone(raw string) (string, error) {
	phone := strings.TrimSpace(raw)
	if !phonePattern.MatchString(phone) {
		return "", serviceerrors.NewValidation("phone", "must be E.164 format (e.g. +628123456789)")
	}
	return phone, nil
}

func validateMarketingStatus(status MarketingStatus) error {
	switch status {
	case MarketingSubscribed, MarketingUnsubscribed, MarketingUnknown, "":
		return nil
	default:
		return serviceerrors.NewValidation("marketing_status", "invalid value")
	}
}

func (s *Service) enrichContact(ctx context.Context, teamID uuid.UUID, c *Contact) (*Contact, error) {
	if c == nil {
		return nil, nil
	}
	c.Tags = []TagRef{}
	tagsByContact, err := s.contacts.ListTagsByContactIDs(ctx, teamID, []uuid.UUID{c.ID})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if tags, ok := tagsByContact[c.ID]; ok {
		c.Tags = tags
	}
	return c, nil
}

func (s *Service) enrichContacts(ctx context.Context, teamID uuid.UUID, contacts []Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(contacts))
	for i := range contacts {
		ids[i] = contacts[i].ID
		contacts[i].Tags = []TagRef{}
	}
	tagsByContact, err := s.contacts.ListTagsByContactIDs(ctx, teamID, ids)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	for i := range contacts {
		if tags, ok := tagsByContact[contacts[i].ID]; ok {
			contacts[i].Tags = tags
		}
	}
	return nil
}
