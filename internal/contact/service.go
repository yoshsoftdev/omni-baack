package contact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
)

var (
	ErrForbidden      = errors.New("insufficient permissions")
	ErrNameRequired   = errors.New("contact name is required")
	ErrPhoneRequired  = errors.New("phone is required")
	ErrPhoneInvalid   = errors.New("invalid phone number")
	ErrInvalidTag     = errors.New("invalid tag")
	ErrEmptyImport    = errors.New("no rows to import")
)

type Service struct {
	contacts Repository
	tags     tag.Repository
	teams    team.Repository
}

func NewService(contacts Repository, tags tag.Repository, teams team.Repository) *Service {
	return &Service{contacts: contacts, tags: tags, teams: teams}
}

type CreateInput struct {
	Name   string
	Phone  string
	Email  string
	Notes  string
	TagIDs []uuid.UUID
}

type UpdateInput struct {
	Name   string
	Phone  string
	Email  string
	Notes  string
	TagIDs []uuid.UUID
}

type ListInput struct {
	Search string
	TagID  *uuid.UUID
	Limit  int
	Offset int
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, input ListInput) ([]Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	contacts, err := s.contacts.ListByTeam(ctx, teamID, ListFilter{
		Search: input.Search,
		TagID:  input.TagID,
		Limit:  input.Limit,
		Offset: input.Offset,
	})
	if err != nil {
		return nil, err
	}

	responses := make([]Response, 0, len(contacts))
	for i := range contacts {
		tags, err := s.contacts.ListTags(ctx, contacts[i].ID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, ToResponse(&contacts[i], tags))
	}
	return responses, nil
}

func (s *Service) Get(ctx context.Context, teamID, userID, contactID uuid.UUID) (*Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	contact, err := s.contacts.FindByID(ctx, teamID, contactID)
	if err != nil {
		return nil, err
	}
	tags, err := s.contacts.ListTags(ctx, contact.ID)
	if err != nil {
		return nil, err
	}
	resp := ToResponse(contact, tags)
	return &resp, nil
}

func (s *Service) Create(ctx context.Context, teamID, userID uuid.UUID, role team.Role, input CreateInput) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	contact, err := s.buildContact(teamID, input.Name, input.Phone, input.Email, input.Notes)
	if err != nil {
		return nil, err
	}

	if err := s.contacts.Create(ctx, contact); err != nil {
		return nil, err
	}
	if err := s.assignTags(ctx, teamID, contact.ID, input.TagIDs); err != nil {
		return nil, err
	}

	tags, err := s.contacts.ListTags(ctx, contact.ID)
	if err != nil {
		return nil, err
	}
	resp := ToResponse(contact, tags)
	return &resp, nil
}

func (s *Service) Update(ctx context.Context, teamID, userID uuid.UUID, role team.Role, contactID uuid.UUID, input UpdateInput) (*Response, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}

	contact, err := s.contacts.FindByID(ctx, teamID, contactID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrNameRequired
	}
	normalizedPhone := phone.Normalize(input.Phone)
	if normalizedPhone == "" {
		return nil, ErrPhoneRequired
	}
	if !phone.IsValid(normalizedPhone) {
		return nil, ErrPhoneInvalid
	}

	contact.Name = name
	contact.Phone = normalizedPhone
	contact.Email = optionalString(input.Email)
	contact.Notes = optionalString(input.Notes)
	contact.UpdatedAt = time.Now().UTC()

	if err := s.contacts.Update(ctx, contact); err != nil {
		return nil, err
	}
	if err := s.assignTags(ctx, teamID, contact.ID, input.TagIDs); err != nil {
		return nil, err
	}

	tags, err := s.contacts.ListTags(ctx, contact.ID)
	if err != nil {
		return nil, err
	}
	resp := ToResponse(contact, tags)
	return &resp, nil
}

func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, role team.Role, contactID uuid.UUID) error {
	if !canManage(role) {
		return ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return s.contacts.SoftDelete(ctx, teamID, contactID)
}

func (s *Service) ImportRows(ctx context.Context, teamID, userID uuid.UUID, role team.Role, rows []ImportRow) (*ImportResult, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrEmptyImport
	}

	result := &ImportResult{}
	for i, row := range rows {
		name := strings.TrimSpace(row.Name)
		normalizedPhone := phone.Normalize(row.Phone)
		if name == "" || normalizedPhone == "" {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: name and phone are required", i+1))
			continue
		}
		if !phone.IsValid(normalizedPhone) {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: invalid phone", i+1))
			continue
		}

		existing, err := s.contacts.FindByPhone(ctx, teamID, normalizedPhone)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}

		now := time.Now().UTC()
		if existing != nil {
			existing.Name = name
			existing.Email = optionalString(row.Email)
			existing.Notes = optionalString(row.Notes)
			existing.UpdatedAt = now
			if err := s.contacts.Update(ctx, existing); err != nil {
				result.Skipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: %s", i+1, err.Error()))
				continue
			}
			result.Updated++
			continue
		}

		contact := &Contact{
			ID:        uuid.New(),
			TeamID:    teamID,
			Name:      name,
			Phone:     normalizedPhone,
			Email:     optionalString(row.Email),
			Notes:     optionalString(row.Notes),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.contacts.Create(ctx, contact); err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: %s", i+1, err.Error()))
			continue
		}
		result.Created++
	}
	return result, nil
}

func (s *Service) buildContact(teamID uuid.UUID, name, rawPhone, email, notes string) (*Contact, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	normalizedPhone := phone.Normalize(rawPhone)
	if normalizedPhone == "" {
		return nil, ErrPhoneRequired
	}
	if !phone.IsValid(normalizedPhone) {
		return nil, ErrPhoneInvalid
	}

	now := time.Now().UTC()
	return &Contact{
		ID:        uuid.New(),
		TeamID:    teamID,
		Name:      name,
		Phone:     normalizedPhone,
		Email:     optionalString(email),
		Notes:     optionalString(notes),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Service) assignTags(ctx context.Context, teamID, contactID uuid.UUID, tagIDs []uuid.UUID) error {
	if len(tagIDs) == 0 {
		return s.contacts.SetTags(ctx, contactID, nil)
	}
	for _, tagID := range tagIDs {
		if _, err := s.tags.FindByID(ctx, teamID, tagID); err != nil {
			return ErrInvalidTag
		}
	}
	return s.contacts.SetTags(ctx, contactID, tagIDs)
}

// FindOrCreateByPhone is used by the team inbox to auto-create contacts from WhatsApp inbound messages.
func (s *Service) FindOrCreateByPhone(ctx context.Context, teamID uuid.UUID, rawPhone, displayName string) (*Contact, error) {
	normalizedPhone := phone.Normalize(rawPhone)
	if normalizedPhone == "" || !phone.IsValid(normalizedPhone) {
		return nil, ErrPhoneInvalid
	}

	existing, err := s.contacts.FindByPhone(ctx, teamID, normalizedPhone)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = normalizedPhone
	}

	now := time.Now().UTC()
	contact := &Contact{
		ID:        uuid.New(),
		TeamID:    teamID,
		Name:      name,
		Phone:     normalizedPhone,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.contacts.Create(ctx, contact); err != nil {
		return nil, err
	}
	return contact, nil
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
