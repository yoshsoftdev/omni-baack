package provider

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

var providerKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

func validateKind(kind Kind) error {
	if !providerKeyPattern.MatchString(string(kind)) {
		return serviceerrors.NewValidation("provider", "unsupported provider")
	}
	return nil
}

func validateStatus(status Status) error {
	switch status {
	case StatusActive, StatusUnavailable, StatusUnlinked:
		return nil
	default:
		return serviceerrors.NewValidation("status", "invalid value")
	}
}

func validateProfile(profile *Profile) error {
	if profile == nil {
		return serviceerrors.NewValidation("profile", "is required")
	}
	if err := validateKind(profile.Provider); err != nil {
		return err
	}
	if strings.TrimSpace(profile.ProviderUserID) == "" {
		return serviceerrors.NewValidation("provider_user_id", "is required")
	}
	if profile.TeamID == uuid.Nil || profile.ContactID == uuid.Nil {
		return serviceerrors.NewValidation("ownership", "team_id and contact_id are required")
	}
	return validateStatus(profile.Status)
}
