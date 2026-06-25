package channel

import (
	"strings"
	"time"

	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// ValidateForMeta ensures a channel is ready for Meta API operations.
func ValidateForMeta(ch *Channel, creds *Credentials, accessToken string) error {
	if ch == nil {
		return serviceerrors.NewValidation("channel_id", "channel not found")
	}
	if ch.Status != StatusConnected {
		return serviceerrors.NewValidation("channel_id", "channel must be connected")
	}
	if ch.WABAID == nil || strings.TrimSpace(*ch.WABAID) == "" {
		return serviceerrors.NewValidation("channel_id", "waba_id is required")
	}
	if ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
		return serviceerrors.NewValidation("channel_id", "phone_number_id is required")
	}
	if creds == nil || len(creds.AccessTokenEnc) == 0 {
		return serviceerrors.NewValidation("channel_id", "access token is required")
	}
	if strings.TrimSpace(accessToken) == "" {
		return serviceerrors.NewValidation("channel_id", "access token is required")
	}
	if creds.TokenExpiresAt != nil && creds.TokenExpiresAt.Before(time.Now().UTC()) {
		return serviceerrors.NewValidation("channel_id", "access token has expired")
	}
	return nil
}

// ValidateForMetaWithOnboarding includes onboarding completion in readiness checks.
func ValidateForMetaWithOnboarding(ch *Channel, ob *Onboarding, creds *Credentials, accessToken string) error {
	if err := ValidateForMeta(ch, creds, accessToken); err != nil {
		return err
	}
	if ob == nil || ob.Status != OnboardingCompleted {
		return serviceerrors.NewValidation("channel_id", "channel onboarding must be completed")
	}
	return nil
}
