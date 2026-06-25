package channel

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// ManualConnectMetaClient validates manual credentials against Meta Graph API.
type ManualConnectMetaClient interface {
	Ping(ctx context.Context, accessToken string) error
	GetWABA(ctx context.Context, wabaID, accessToken string) (*metapkg.WABA, error)
	GetPhoneNumber(ctx context.Context, phoneNumberID, accessToken string) (*metapkg.PhoneNumber, error)
	GetPhoneNumbers(ctx context.Context, wabaID, accessToken string) ([]metapkg.PhoneNumber, error)
	ListTemplates(ctx context.Context, wabaID, accessToken string) ([]metapkg.MessageTemplate, error)
}

type ManualVerifyChecks struct {
	Token        bool `json:"token"`
	WABA         bool `json:"waba"`
	Phone        bool `json:"phone"`
	TemplatesAPI bool `json:"templates_api"`
}

type ManualVerifyResult struct {
	WABAName      string             `json:"waba_name,omitempty"`
	PhoneDisplay  string             `json:"phone_display,omitempty"`
	TemplateCount int                `json:"template_count"`
	Checks        ManualVerifyChecks `json:"checks"`
}

func normalizeManualInput(input ManualConnectInput) (token, wabaID, phoneNumberID string, err error) {
	token = strings.TrimSpace(input.AccessToken)
	wabaID = strings.TrimSpace(input.WABAID)
	phoneNumberID = strings.TrimSpace(input.PhoneNumberID)
	if token == "" || wabaID == "" || phoneNumberID == "" {
		return "", "", "", serviceerrors.NewValidation("manual_connect", "access_token, waba_id, and phone_number_id are required")
	}
	return token, wabaID, phoneNumberID, nil
}

func (s *Service) VerifyManualConnect(ctx context.Context, teamID, userID uuid.UUID, actorRole string, input ManualConnectInput) (*ManualVerifyResult, error) {
	if err := s.authorize(actorRole, rbac.PermDeviceManage); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return s.verifyManualConnect(ctx, input)
}

func (s *Service) verifyManualConnect(ctx context.Context, input ManualConnectInput) (*ManualVerifyResult, error) {
	token, wabaID, phoneNumberID, err := normalizeManualInput(input)
	if err != nil {
		return nil, err
	}
	if s.meta == nil {
		return nil, serviceerrors.NewValidation("manual_connect", "Meta API verification is not configured")
	}

	result := &ManualVerifyResult{Checks: ManualVerifyChecks{}}

	if err := s.meta.Ping(ctx, token); err != nil {
		return nil, serviceerrors.NewValidation("access_token", "invalid or expired Meta access token")
	}
	result.Checks.Token = true

	waba, err := s.meta.GetWABA(ctx, wabaID, token)
	if err != nil {
		return nil, mapManualMetaError("waba_id", err, "invalid WhatsApp Business Account ID")
	}
	result.Checks.WABA = true
	result.WABAName = waba.Name

	phone, err := s.meta.GetPhoneNumber(ctx, phoneNumberID, token)
	if err != nil {
		return nil, mapManualMetaError("phone_number_id", err, "invalid Phone Number ID")
	}
	result.Checks.Phone = true
	result.PhoneDisplay = phone.DisplayPhoneNumber

	if phones, listErr := s.meta.GetPhoneNumbers(ctx, wabaID, token); listErr == nil {
		if !phoneBelongsToWABA(phones, phoneNumberID) {
			return nil, serviceerrors.NewValidation("phone_number_id", "phone number does not belong to the specified WABA")
		}
	}

	templates, err := s.meta.ListTemplates(ctx, wabaID, token)
	if err != nil {
		return nil, mapManualMetaError("waba_id", err, "cannot access message templates for this WABA; verify WABA ID and token permissions")
	}
	result.Checks.TemplatesAPI = true
	result.TemplateCount = len(templates)

	return result, nil
}

func phoneBelongsToWABA(phones []metapkg.PhoneNumber, phoneNumberID string) bool {
	for _, phone := range phones {
		if phone.ID == phoneNumberID {
			return true
		}
	}
	return false
}

func mapManualMetaError(field string, err error, fallback string) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "nonexisting field") && strings.Contains(lower, "message_templates"):
		return serviceerrors.NewValidation("waba_id", "invalid WhatsApp Business Account ID; use WABA ID from Meta Business Manager, not Phone Number ID")
	case strings.Contains(lower, "oauth") || strings.Contains(lower, "session has expired") || strings.Contains(lower, "invalid oauth"):
		return serviceerrors.NewValidation("access_token", "invalid or expired Meta access token")
	case strings.Contains(lower, "unsupported get request") || strings.Contains(lower, "does not exist"):
		return serviceerrors.NewValidation(field, fallback)
	default:
		if msg != "" {
			return serviceerrors.NewValidation(field, fallback+": "+msg)
		}
		return serviceerrors.NewValidation(field, fallback)
	}
}
