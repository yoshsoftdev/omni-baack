package automation

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type TemplateFinder interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*template.Template, error)
}

type ChannelFinder interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*channel.Channel, error)
}

type Service struct {
	repo      Repository
	teams     team.Repository
	templates TemplateFinder
	channels  ChannelFinder
	rbac      *rbac.Service
	invoker   *Invoker
	forwarder *Forwarder
}

func NewService(repo Repository, teams team.Repository, templates TemplateFinder, channels ChannelFinder, rbacSvc *rbac.Service, invoker *Invoker) *Service {
	return &Service{
		repo:      repo,
		teams:     teams,
		templates: templates,
		channels:  channels,
		rbac:      rbacSvc,
		invoker:   invoker,
		forwarder: NewForwarder(repo),
	}
}

func (s *Service) Invoker() *Invoker {
	return s.invoker
}

func (s *Service) Forwarder() *Forwarder {
	return s.forwarder
}

func (s *Service) authorize(role, perm string) error {
	if s.rbac != nil && s.rbac.Require(role, perm) {
		return nil
	}
	if s.rbac == nil && role != "" {
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

func (s *Service) GetSummary(ctx context.Context, teamID, userID uuid.UUID, role string) (*Summary, error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.repo.GetSummary(ctx, teamID)
}

func (s *Service) ListEndpoints(ctx context.Context, teamID, userID uuid.UUID, role string, filter EndpointListFilter, page repository.PageParams) (repository.Page[Endpoint], error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return repository.Page[Endpoint]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[Endpoint]{}, err
	}
	result, err := s.repo.ListEndpoints(ctx, teamID, filter, page)
	if err != nil {
		return repository.Page[Endpoint]{}, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) GetEndpoint(ctx context.Context, teamID, userID uuid.UUID, role string, id string) (*Endpoint, error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindEndpointByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return item, nil
}

func (s *Service) CreateEndpoint(ctx context.Context, teamID, userID uuid.UUID, role string, input CreateEndpointInput) (*Endpoint, error) {
	if err := s.authorize(role, rbac.PermAutomationCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, serviceerrors.NewValidation("name", "is required")
	}
	slug := Slugify(input.Slug)
	if slug == "" {
		slug = Slugify(name)
	}
	if !IsValidSlug(slug) {
		return nil, serviceerrors.NewValidation("slug", "must be kebab-case")
	}
	actionType := input.ActionType
	if actionType == "" {
		actionType = ActionSendTemplate
	}
	rawCfg, err := s.parseAndValidateActionConfig(ctx, teamID, actionType, input.ActionConfig)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.SlugExists(ctx, teamID, slug, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, serviceerrors.NewValidation("slug", "already exists")
	}
	webhookCfg, err := parseWebhookConfig(input.Webhook)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	item := &Endpoint{
		ID:           NewEndpointID(),
		TeamID:       teamID,
		Name:         name,
		Slug:         slug,
		Description:  strings.TrimSpace(input.Description),
		Method:       "POST",
		Status:       EndpointStatusActive,
		ActionType:   actionType,
		ActionConfig: rawCfg,
		Webhook:      webhookCfg,
		CreatedBy:    &userID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Stats:        EndpointStats{SuccessRate: 100},
	}
	if err := serviceerrors.MapRepository(s.repo.CreateEndpoint(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) UpdateEndpoint(ctx context.Context, teamID, userID uuid.UUID, role string, id string, input UpdateEndpointInput) (*Endpoint, error) {
	if err := s.authorize(role, rbac.PermAutomationUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindEndpointByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		item.Name = name
	}
	if input.Slug != nil {
		slug := Slugify(*input.Slug)
		if !IsValidSlug(slug) {
			return nil, serviceerrors.NewValidation("slug", "must be kebab-case")
		}
		exists, err := s.repo.SlugExists(ctx, teamID, slug, item.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, serviceerrors.NewValidation("slug", "already exists")
		}
		item.Slug = slug
	}
	if input.Description != nil {
		item.Description = strings.TrimSpace(*input.Description)
	}
	if input.Status != nil {
		item.Status = *input.Status
	}
	if input.Webhook != nil {
		webhookCfg, err := parseWebhookConfig(input.Webhook)
		if err != nil {
			return nil, err
		}
		item.Webhook = webhookCfg
	}
	if len(input.ActionConfig) > 0 {
		rawCfg, err := s.parseAndValidateActionConfig(ctx, teamID, item.ActionType, input.ActionConfig)
		if err != nil {
			return nil, err
		}
		item.ActionConfig = rawCfg
	}
	item.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.repo.UpdateEndpoint(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) SetEndpointStatus(ctx context.Context, teamID, userID uuid.UUID, role string, id string, status EndpointStatus) (*Endpoint, error) {
	return s.UpdateEndpoint(ctx, teamID, userID, role, id, UpdateEndpointInput{Status: &status})
}

func (s *Service) DuplicateEndpoint(ctx context.Context, teamID, userID uuid.UUID, role string, id string) (*Endpoint, error) {
	if err := s.authorize(role, rbac.PermAutomationCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	source, err := s.repo.FindEndpointByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	slug := source.Slug + "-copy"
	base := slug
	for i := 2; ; i++ {
		exists, err := s.repo.SlugExists(ctx, teamID, slug, "")
		if err != nil {
			return nil, err
		}
		if !exists {
			break
		}
		slug = base + "-" + strings.TrimSpace(strings.ReplaceAll(uuid.New().String()[:4], "-", ""))
		if i > 10 {
			slug = Slugify(base + "-" + uuid.New().String()[:8])
			break
		}
	}
	return s.CreateEndpoint(ctx, teamID, userID, role, CreateEndpointInput{
		Name:         source.Name + " (Copy)",
		Slug:         slug,
		Description:  source.Description,
		ActionType:   source.ActionType,
		ActionConfig: source.ActionConfig,
	})
}

func (s *Service) DeleteEndpoint(ctx context.Context, teamID, userID uuid.UUID, role string, id string) error {
	if err := s.authorize(role, rbac.PermAutomationDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.repo.SoftDeleteEndpoint(ctx, teamID, id))
}

func (s *Service) GetAnalytics(ctx context.Context, teamID, userID uuid.UUID, role string, id string) (*EndpointAnalytics, error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.repo.GetEndpointAnalytics(ctx, teamID, id)
}

func (s *Service) ListAPIKeys(ctx context.Context, teamID, userID uuid.UUID, role string) ([]APIKey, error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListAPIKeys(ctx, teamID)
}

func (s *Service) CreateAPIKey(ctx context.Context, teamID, userID uuid.UUID, role string, input CreateAPIKeyInput) (*APIKeySecretResult, error) {
	if err := s.authorize(role, rbac.PermAutomationCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, serviceerrors.NewValidation("name", "is required")
	}
	scopes := normalizeScopes(input.Scopes)
	secret, prefix, hash, err := GenerateAPIKeySecret()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	item := &APIKey{
		ID:        NewAPIKeyID(),
		TeamID:    teamID,
		Name:      name,
		KeyPrefix: prefix,
		KeyHash:   hash,
		Scopes:    scopes,
		Status:    APIKeyStatusActive,
		CreatedBy: &userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := serviceerrors.MapRepository(s.repo.CreateAPIKey(ctx, item)); err != nil {
		return nil, err
	}
	return &APIKeySecretResult{Key: item, Secret: secret}, nil
}

func (s *Service) RotateAPIKey(ctx context.Context, teamID, userID uuid.UUID, role string, id string) (*APIKeySecretResult, error) {
	if err := s.authorize(role, rbac.PermAutomationUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindAPIKeyByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	secret, prefix, hash, err := GenerateAPIKeySecret()
	if err != nil {
		return nil, err
	}
	item.KeyPrefix = prefix
	item.KeyHash = hash
	item.Status = APIKeyStatusActive
	item.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.repo.UpdateAPIKey(ctx, item)); err != nil {
		return nil, err
	}
	return &APIKeySecretResult{Key: item, Secret: secret}, nil
}

func (s *Service) SetAPIKeyStatus(ctx context.Context, teamID, userID uuid.UUID, role string, id string, status APIKeyStatus) (*APIKey, error) {
	if err := s.authorize(role, rbac.PermAutomationUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindAPIKeyByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	item.Status = status
	item.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.repo.UpdateAPIKey(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) DeleteAPIKey(ctx context.Context, teamID, userID uuid.UUID, role string, id string) error {
	if err := s.authorize(role, rbac.PermAutomationDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.repo.SoftDeleteAPIKey(ctx, teamID, id))
}

func (s *Service) TestInvoke(ctx context.Context, teamID, userID uuid.UUID, role string, endpointID string, body map[string]any, clientIP string) (*InvokeResult, error) {
	if err := s.authorize(role, rbac.PermAutomationInvoke); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if s.invoker == nil {
		return nil, serviceerrors.NewValidation("invoke", "path is not configured")
	}
	ep, err := s.repo.FindEndpointByID(ctx, teamID, endpointID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return s.invoker.Invoke(ctx, ep, nil, body, clientIP)
}

func parseAndValidateSendTemplateConfig(raw json.RawMessage) (*SendTemplateActionConfig, error) {
	if len(raw) == 0 {
		return nil, serviceerrors.NewValidation("actionConfig", "is required")
	}
	var cfg SendTemplateActionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, serviceerrors.NewValidation("actionConfig", "is invalid")
	}
	if strings.TrimSpace(cfg.TemplateID) == "" {
		return nil, serviceerrors.NewValidation("actionConfig.templateId", "is required")
	}
	if cfg.RecipientMode == "" {
		cfg.RecipientMode = RecipientModePhone
	}
	if cfg.RecipientMode != RecipientModePhone {
		return nil, serviceerrors.NewValidation("actionConfig.recipientMode", "only phone is supported")
	}
	seen := map[string]struct{}{}
	for i := range cfg.VariableMappings {
		m := &cfg.VariableMappings[i]
		m.APIParameter = strings.TrimSpace(m.APIParameter)
		m.TemplateVariable = strings.TrimSpace(m.TemplateVariable)
		if m.TemplateVariable == "" || m.APIParameter == "" {
			return nil, serviceerrors.NewValidation("actionConfig.variableMappings", "templateVariable and apiParameter are required")
		}
		if _, ok := seen[m.APIParameter]; ok {
			return nil, serviceerrors.NewValidation("actionConfig.variableMappings", "apiParameter names must be unique")
		}
		seen[m.APIParameter] = struct{}{}
	}
	return &cfg, nil
}

func parseAndValidateSendTextConfig(raw json.RawMessage) (*SendTextActionConfig, error) {
	if len(raw) == 0 {
		return nil, serviceerrors.NewValidation("actionConfig", "is required")
	}
	var cfg SendTextActionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, serviceerrors.NewValidation("actionConfig", "is invalid")
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return nil, serviceerrors.NewValidation("actionConfig.channelId", "is required")
	}
	if cfg.RecipientMode == "" {
		cfg.RecipientMode = RecipientModePhone
	}
	if cfg.RecipientMode != RecipientModePhone {
		return nil, serviceerrors.NewValidation("actionConfig.recipientMode", "only phone is supported")
	}
	cfg.TextParameter = strings.TrimSpace(cfg.TextParameter)
	if cfg.TextParameter == "" {
		cfg.TextParameter = "text"
	}
	cfg.DefaultText = strings.TrimSpace(cfg.DefaultText)
	return &cfg, nil
}

func (s *Service) parseAndValidateActionConfig(
	ctx context.Context,
	teamID uuid.UUID,
	actionType ActionType,
	raw json.RawMessage,
) (json.RawMessage, error) {
	switch actionType {
	case ActionSendTemplate:
		cfg, err := parseAndValidateSendTemplateConfig(raw)
		if err != nil {
			return nil, err
		}
		if s.templates != nil {
			tplID, err := uuid.Parse(cfg.TemplateID)
			if err != nil {
				return nil, serviceerrors.NewValidation("actionConfig.templateId", "is invalid")
			}
			tpl, err := s.templates.FindByID(ctx, teamID, tplID)
			if err != nil {
				return nil, serviceerrors.MapRepository(err)
			}
			if tpl.Status != template.StatusApproved {
				return nil, serviceerrors.NewValidation("actionConfig.templateId", "template must be approved")
			}
			if cfg.TemplateName == "" {
				cfg.TemplateName = tpl.Name
			}
		}
		return json.Marshal(cfg)
	case ActionSendText:
		cfg, err := parseAndValidateSendTextConfig(raw)
		if err != nil {
			return nil, err
		}
		channelID, err := uuid.Parse(cfg.ChannelID)
		if err != nil {
			return nil, serviceerrors.NewValidation("actionConfig.channelId", "is invalid")
		}
		if s.channels != nil {
			ch, err := s.channels.FindByID(ctx, teamID, channelID)
			if err != nil {
				return nil, serviceerrors.MapRepository(err)
			}
			if ch.ChannelType != channel.TypeWhatsApp {
				return nil, serviceerrors.NewValidation("actionConfig.channelId", "must be a WhatsApp channel")
			}
		}
		return json.Marshal(cfg)
	default:
		return nil, serviceerrors.NewValidation("actionType", "only send_template and send_text are supported")
	}
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{"send_template"}
	}
	allowed := map[string]struct{}{
		"send_template":    {},
		"send_text":        {},
		"endpoints.invoke": {},
	}
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := allowed[scope]; !ok {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return []string{"send_template"}
	}
	return out
}

func parseWebhookConfig(in *WebhookConfig) (WebhookConfig, error) {
	if in == nil {
		return WebhookConfig{}, nil
	}
	out := WebhookConfig{
		URL:       strings.TrimSpace(in.URL),
		IsEnabled: in.IsEnabled,
		Secret:    strings.TrimSpace(in.Secret),
	}
	if out.URL == "" {
		if out.IsEnabled {
			return WebhookConfig{}, serviceerrors.NewValidation("webhook.url", "is required when enabled")
		}
		return WebhookConfig{}, nil
	}
	parsed, err := url.Parse(out.URL)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Host) == "" {
		return WebhookConfig{}, serviceerrors.NewValidation("webhook.url", "must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return WebhookConfig{}, serviceerrors.NewValidation("webhook.url", "must use http or https")
	}
	if out.Secret == "" {
		out.Secret = strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	return out, nil
}

func (s *Service) ListForwardWebhooks(ctx context.Context, teamID, userID uuid.UUID, role string) ([]ForwardWebhook, error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListForwardWebhooks(ctx, teamID)
}

func (s *Service) CreateForwardWebhook(ctx context.Context, teamID, userID uuid.UUID, role string, input CreateForwardWebhookInput) (*ForwardWebhook, error) {
	if err := s.authorize(role, rbac.PermAutomationCreate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, serviceerrors.NewValidation("name", "is required")
	}
	urlValue, err := validateForwardURL(input.URL)
	if err != nil {
		return nil, err
	}
	events := normalizeForwardEvents(input.Events)
	secret := strings.TrimSpace(input.Secret)
	if secret == "" {
		secret = strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	if input.ChannelID != nil && s.channels != nil {
		if _, err := s.channels.FindByID(ctx, teamID, *input.ChannelID); err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
	}
	now := time.Now().UTC()
	item := &ForwardWebhook{
		ID:        NewForwardWebhookID(),
		TeamID:    teamID,
		Name:      name,
		URL:       urlValue,
		Secret:    secret,
		Status:    ForwardWebhookActive,
		Events:    events,
		ChannelID: input.ChannelID,
		CreatedBy: &userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := serviceerrors.MapRepository(s.repo.CreateForwardWebhook(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) UpdateForwardWebhook(ctx context.Context, teamID, userID uuid.UUID, role string, id string, input UpdateForwardWebhookInput) (*ForwardWebhook, error) {
	if err := s.authorize(role, rbac.PermAutomationUpdate); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindForwardWebhookByID(ctx, teamID, id)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, serviceerrors.NewValidation("name", "is required")
		}
		item.Name = name
	}
	if input.URL != nil {
		urlValue, err := validateForwardURL(*input.URL)
		if err != nil {
			return nil, err
		}
		item.URL = urlValue
	}
	if input.Status != nil {
		item.Status = *input.Status
	}
	if input.Events != nil {
		item.Events = normalizeForwardEvents(input.Events)
	}
	if input.ClearChannel {
		item.ChannelID = nil
	} else if input.ChannelID != nil {
		if s.channels != nil {
			if _, err := s.channels.FindByID(ctx, teamID, *input.ChannelID); err != nil {
				return nil, serviceerrors.MapRepository(err)
			}
		}
		item.ChannelID = input.ChannelID
	}
	if input.RotateSecret {
		item.Secret = strings.ReplaceAll(uuid.New().String(), "-", "")
	} else if input.Secret != nil {
		secret := strings.TrimSpace(*input.Secret)
		if secret != "" {
			item.Secret = secret
		}
	}
	item.UpdatedAt = time.Now().UTC()
	if err := serviceerrors.MapRepository(s.repo.UpdateForwardWebhook(ctx, item)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) SetForwardWebhookStatus(ctx context.Context, teamID, userID uuid.UUID, role string, id string, status ForwardWebhookStatus) (*ForwardWebhook, error) {
	return s.UpdateForwardWebhook(ctx, teamID, userID, role, id, UpdateForwardWebhookInput{Status: &status})
}

func (s *Service) DeleteForwardWebhook(ctx context.Context, teamID, userID uuid.UUID, role string, id string) error {
	if err := s.authorize(role, rbac.PermAutomationDelete); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.repo.SoftDeleteForwardWebhook(ctx, teamID, id))
}

func (s *Service) ListForwardDeliveries(ctx context.Context, teamID, userID uuid.UUID, role string, webhookID string, page repository.PageParams) (repository.Page[ForwardDelivery], error) {
	if err := s.authorize(role, rbac.PermAutomationView); err != nil {
		return repository.Page[ForwardDelivery]{}, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return repository.Page[ForwardDelivery]{}, err
	}
	if _, err := s.repo.FindForwardWebhookByID(ctx, teamID, webhookID); err != nil {
		return repository.Page[ForwardDelivery]{}, serviceerrors.MapRepository(err)
	}
	return s.repo.ListForwardDeliveries(ctx, teamID, webhookID, page)
}

func validateForwardURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", serviceerrors.NewValidation("url", "is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Host) == "" {
		return "", serviceerrors.NewValidation("url", "must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", serviceerrors.NewValidation("url", "must use http or https")
	}
	return value, nil
}

func normalizeForwardEvents(events []string) []string {
	allowed := map[string]struct{}{
		EventMessageReceived: {},
	}
	out := make([]string, 0, len(events))
	seen := map[string]struct{}{}
	for _, event := range events {
		event = strings.TrimSpace(event)
		if _, ok := allowed[event]; !ok {
			continue
		}
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		out = append(out, event)
	}
	if len(out) == 0 {
		return []string{EventMessageReceived}
	}
	return out
}
