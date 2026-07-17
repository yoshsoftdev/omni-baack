package message

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/shared/mediatoken"
)

type mediaURLService struct {
	issuer            *mediatoken.Issuer
	publicWebhookBase string
}

func NewMediaURLService(secret, publicWebhookBase string, ttl time.Duration) *mediaURLService {
	return &mediaURLService{
		issuer:            mediatoken.NewIssuer(secret, ttl),
		publicWebhookBase: strings.TrimSpace(publicWebhookBase),
	}
}

func (s *mediaURLService) Issue(teamID, objectID uuid.UUID) (string, error) {
	if s == nil || s.issuer == nil {
		return "", fmt.Errorf("media url service is not configured")
	}
	if s.publicWebhookBase == "" {
		return "", fmt.Errorf("WEBHOOK_PUBLIC_BASE_URL is required for Instagram media send")
	}
	token, _, err := s.issuer.Mint(objectID, teamID)
	if err != nil {
		return "", err
	}
	url := mediatoken.BuildWebhookURL(s.publicWebhookBase, token)
	if err := validateOutboundMediaURL(url); err != nil {
		return "", err
	}
	return url, nil
}

func validateOutboundMediaURL(raw string) error {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "://localhost") ||
		strings.Contains(lower, "://127.0.0.1") ||
		strings.Contains(lower, "://[::1]") {
		return fmt.Errorf("WEBHOOK_PUBLIC_BASE_URL must be publicly reachable by Meta (not localhost)")
	}
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return fmt.Errorf("WEBHOOK_PUBLIC_BASE_URL must be an absolute http(s) URL")
	}
	return nil
}
