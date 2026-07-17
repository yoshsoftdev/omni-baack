package contact

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
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
	providers provider.Repository
	links     LinkRepository
}

func (s *Service) SetProviderRepository(providers provider.Repository) {
	s.providers = providers
}

func (s *Service) SetLinkRepository(links LinkRepository) {
	s.links = links
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
	ContactName     string
	CustomerName    *string
	Phone           string
	Email           *string
	Notes           *string
	City            *string
	Country         *string
	Province        *string
	Company         *string
	JobTitle        *string
	Language        *string
	CustomerType    *string
	Source          Source
	MarketingStatus MarketingStatus
	TagIDs          []uuid.UUID
	CustomFields    map[string]json.RawMessage
}

type UpdateInput struct {
	Name            *string
	ContactName     *string
	CustomerName    *string
	Phone           *string
	Email           *string
	Notes           *string
	City            *string
	Country         *string
	Province        *string
	Company         *string
	JobTitle        *string
	Language        *string
	CustomerType    *string
	MarketingStatus *MarketingStatus
	TagIDs          []uuid.UUID
	TagIDsSet       bool
	CustomFields    map[string]json.RawMessage
	CustomFieldsSet bool
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

	contactName, customerName, err := resolveCreateNames(input)
	if err != nil {
		return nil, err
	}

	source := input.Source
	if source == "" {
		source = SourceManual
	}

	now := time.Now().UTC()
	country, err := normalizeCountryPtr(input.Country)
	if err != nil {
		return nil, err
	}
	c := &Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		ContactName:     contactName,
		CustomerName:    customerName,
		Phone:           phone,
		Email:           input.Email,
		Notes:           input.Notes,
		City:            input.City,
		Country:         country,
		Province:        input.Province,
		Company:         input.Company,
		JobTitle:        input.JobTitle,
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
	if len(input.TagIDs) > 0 {
		if err := serviceerrors.MapRepository(s.contacts.SyncContactTags(ctx, teamID, c.ID, input.TagIDs)); err != nil {
			return nil, err
		}
	}
	if len(input.CustomFields) > 0 {
		if err := s.saveCustomFields(ctx, teamID, c.ID, input.CustomFields); err != nil {
			return nil, err
		}
	}
	return s.enrichContact(ctx, teamID, c, true)
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
	return s.enrichContact(ctx, teamID, c, true)
}

func (s *Service) ListLinks(
	ctx context.Context,
	teamID, userID uuid.UUID,
	actorRole string,
	contactID uuid.UUID,
) ([]LinkedContact, error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, err := s.contacts.FindByID(ctx, teamID, contactID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if s.links == nil {
		return []LinkedContact{}, nil
	}
	items, err := s.links.ListByContactIDs(ctx, teamID, []uuid.UUID{contactID})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if linked, ok := items[contactID]; ok {
		return linked, nil
	}
	return []LinkedContact{}, nil
}

func (s *Service) LinkContact(
	ctx context.Context,
	teamID, userID uuid.UUID,
	actorRole string,
	contactID, linkedContactID uuid.UUID,
) (*LinkedContact, error) {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if contactID == linkedContactID {
		return nil, serviceerrors.NewValidation("contact_id", "cannot link a contact to itself")
	}
	if s.links == nil {
		return nil, errors.New("contact link repository is not configured")
	}
	if _, err := s.contacts.FindByID(ctx, teamID, contactID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if _, err := s.contacts.FindByID(ctx, teamID, linkedContactID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if err := s.links.Link(ctx, teamID, contactID, linkedContactID, userID); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	items, err := s.links.ListByContactIDs(ctx, teamID, []uuid.UUID{contactID})
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	for i := range items[contactID] {
		if items[contactID][i].ID == linkedContactID {
			return &items[contactID][i], nil
		}
	}
	return nil, serviceerrors.MapRepository(repository.ErrNotFound)
}

func (s *Service) UnlinkContact(
	ctx context.Context,
	teamID, userID uuid.UUID,
	actorRole string,
	contactID, linkedContactID uuid.UUID,
) error {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	if contactID == linkedContactID {
		return serviceerrors.NewValidation("linked_contact_id", "cannot unlink a contact from itself")
	}
	if s.links == nil {
		return errors.New("contact link repository is not configured")
	}
	if _, err := s.contacts.FindByID(ctx, teamID, contactID); err != nil {
		return serviceerrors.MapRepository(err)
	}
	if _, err := s.contacts.FindByID(ctx, teamID, linkedContactID); err != nil {
		return serviceerrors.MapRepository(err)
	}
	return serviceerrors.MapRepository(s.links.Unlink(ctx, teamID, contactID, linkedContactID))
}

func (s *Service) Stats(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (map[string]int64, error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	counts, err := s.contacts.CountMarketingByTeam(ctx, teamID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	total := counts[MarketingSubscribed] + counts[MarketingUnsubscribed] + counts[MarketingUnknown]
	return map[string]int64{
		"total":   total,
		"opt_in":  counts[MarketingSubscribed],
		"opt_out": counts[MarketingUnsubscribed],
		"unknown": counts[MarketingUnknown],
	}, nil
}

func (s *Service) ListFieldDefinitions(ctx context.Context, teamID, userID uuid.UUID, actorRole string) ([]FieldDefinition, error) {
	if err := s.authorize(actorRole, rbac.PermContactView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.contacts.ListFieldDefinitions(ctx, teamID)
}

func (s *Service) CreateFieldDefinition(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input CreateFieldDefinitionInput) (*FieldDefinition, error) {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	fieldKey := normalizeFieldKey(input.FieldKey)
	if fieldKey == "" {
		return nil, serviceerrors.NewValidation("field_key", "is required")
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		return nil, serviceerrors.NewValidation("label", "is required")
	}
	if err := validateFieldType(input.FieldType); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	def := &FieldDefinition{
		ID:        uuid.New(),
		TeamID:    teamID,
		FieldKey:  fieldKey,
		Label:     label,
		FieldType: input.FieldType,
		Options:   input.Options,
		SortOrder: input.SortOrder,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := serviceerrors.MapRepository(s.contacts.CreateFieldDefinition(ctx, def)); err != nil {
		return nil, err
	}
	return def, nil
}

func (s *Service) UpdateFieldDefinition(ctx context.Context, teamID, userID uuid.UUID, actorRole string, id uuid.UUID, input UpdateFieldDefinitionInput) (*FieldDefinition, error) {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	def, err := s.contacts.FindFieldDefinitionByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Label != nil {
		label := strings.TrimSpace(*input.Label)
		if label == "" {
			return nil, serviceerrors.NewValidation("label", "is required")
		}
		def.Label = label
	}
	if input.FieldType != nil {
		if err := validateFieldType(*input.FieldType); err != nil {
			return nil, err
		}
		def.FieldType = *input.FieldType
	}
	if input.Options != nil {
		def.Options = input.Options
	}
	if input.SortOrder != nil {
		def.SortOrder = *input.SortOrder
	}
	def.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.contacts.UpdateFieldDefinition(ctx, def)); err != nil {
		return nil, err
	}
	return def, nil
}

func (s *Service) DeleteFieldDefinition(ctx context.Context, teamID, userID uuid.UUID, actorRole string, id uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.contacts.SoftDeleteFieldDefinition(ctx, teamID, id))
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
	if err := s.enrichContacts(ctx, teamID, result.Items, false); err != nil {
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
	if input.ContactName != nil || input.Name != nil {
		contactName, err := resolveUpdateContactName(input)
		if err != nil {
			return nil, err
		}
		c.ContactName = contactName
	}
	if input.CustomerName != nil {
		customerName := strings.TrimSpace(*input.CustomerName)
		if customerName == "" {
			return nil, serviceerrors.NewValidation("customer_name", "is required")
		}
		c.CustomerName = customerName
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
	if input.Country != nil {
		country, err := normalizeCountry(*input.Country)
		if err != nil {
			return nil, err
		}
		c.Country = country
	}
	if input.Province != nil {
		c.Province = input.Province
	}
	if input.Company != nil {
		c.Company = input.Company
	}
	if input.JobTitle != nil {
		c.JobTitle = input.JobTitle
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
	if input.TagIDsSet {
		if err := serviceerrors.MapRepository(s.contacts.SyncContactTags(ctx, teamID, c.ID, input.TagIDs)); err != nil {
			return nil, err
		}
	}
	if input.CustomFieldsSet && len(input.CustomFields) > 0 {
		if err := s.saveCustomFields(ctx, teamID, c.ID, input.CustomFields); err != nil {
			return nil, err
		}
	}
	return s.enrichContact(ctx, teamID, c, true)
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

func normalizeCountry(raw string) (*string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return nil, nil
	}
	if len(value) != 2 {
		return nil, serviceerrors.NewValidation("country", "must be a 2-letter ISO country code (e.g. ID)")
	}
	return &value, nil
}

func normalizeCountryPtr(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	return normalizeCountry(*raw)
}

func validateMarketingStatus(status MarketingStatus) error {
	switch status {
	case MarketingSubscribed, MarketingUnsubscribed, MarketingUnknown, "":
		return nil
	default:
		return serviceerrors.NewValidation("marketing_status", "invalid value")
	}
}

func validateFieldType(fieldType FieldType) error {
	switch fieldType {
	case FieldTypeText, FieldTypeNumber, FieldTypeDate, FieldTypeBoolean, FieldTypeSelect:
		return nil
	default:
		return serviceerrors.NewValidation("field_type", "invalid value")
	}
}

func (s *Service) saveCustomFields(ctx context.Context, teamID, contactID uuid.UUID, values map[string]json.RawMessage) error {
	if len(values) == 0 {
		return nil
	}
	defs, err := s.contacts.ListFieldDefinitions(ctx, teamID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	byKey := make(map[string]uuid.UUID, len(defs))
	for _, def := range defs {
		byKey[def.FieldKey] = def.ID
	}
	payload := make(map[uuid.UUID]json.RawMessage)
	for key, value := range values {
		id, ok := byKey[normalizeFieldKey(key)]
		if !ok {
			continue
		}
		payload[id] = value
	}
	if len(payload) == 0 {
		return nil
	}
	return serviceerrors.MapRepository(s.contacts.UpsertCustomFieldValues(ctx, contactID, payload))
}

func (s *Service) enrichContact(ctx context.Context, teamID uuid.UUID, c *Contact, includeStats bool) (*Contact, error) {
	if c == nil {
		return nil, nil
	}
	items := []Contact{*c}
	if err := s.enrichContacts(ctx, teamID, items, includeStats); err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (s *Service) enrichContacts(ctx context.Context, teamID uuid.UUID, contacts []Contact, includeStats bool) error {
	if len(contacts) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(contacts))
	for i := range contacts {
		ids[i] = contacts[i].ID
		contacts[i].Tags = []TagRef{}
		contacts[i].Providers = []provider.Profile{}
		contacts[i].LinkedContacts = []LinkedContact{}
		contacts[i].CustomFields = []CustomFieldValue{}
	}
	tagsByContact, err := s.contacts.ListTagsByContactIDs(ctx, teamID, ids)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	fieldsByContact, err := s.contacts.ListCustomFieldsByContactIDs(ctx, teamID, ids)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	providersByContact := map[uuid.UUID][]provider.Profile{}
	if s.providers != nil {
		providersByContact, err = s.providers.ListByContactIDs(ctx, teamID, ids)
		if err != nil {
			return serviceerrors.MapRepository(err)
		}
	}
	linksByContact := map[uuid.UUID][]LinkedContact{}
	if s.links != nil {
		linksByContact, err = s.links.ListByContactIDs(ctx, teamID, ids)
		if err != nil {
			return serviceerrors.MapRepository(err)
		}
	}
	var statsByContact map[uuid.UUID]ContactStats
	if includeStats {
		statsByContact, err = s.contacts.LoadContactStats(ctx, teamID, ids)
		if err != nil {
			return serviceerrors.MapRepository(err)
		}
	}
	for i := range contacts {
		if tags, ok := tagsByContact[contacts[i].ID]; ok {
			contacts[i].Tags = tags
		}
		if fields, ok := fieldsByContact[contacts[i].ID]; ok {
			contacts[i].CustomFields = fields
		}
		if profiles, ok := providersByContact[contacts[i].ID]; ok {
			contacts[i].Providers = profiles
		}
		if links, ok := linksByContact[contacts[i].ID]; ok {
			contacts[i].LinkedContacts = links
		}
		if includeStats {
			if stats, ok := statsByContact[contacts[i].ID]; ok {
				contacts[i].ConversationsCount = stats.ConversationsCount
				contacts[i].LastCampaign = stats.LastCampaign
			}
		}
		contacts[i].PopulateLegacyName()
	}
	return nil
}
