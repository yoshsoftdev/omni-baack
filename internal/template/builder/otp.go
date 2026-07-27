package builder

import (
	"fmt"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/template"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// Authentication OTP template identity (Meta-approved AUTHENTICATION template).
const (
	AuthenticationOTPName = "kode_otp"
	// AuthenticationOTPLanguageStored is the domain-stored locale (not Meta send language.code).
	AuthenticationOTPLanguageStored = "id"
)

// BuildAuthenticationOTP renders send components for the kode_otp template.
// Meta Graph DTO construction stays at this package edge — callers must not import pkg/meta builders.
func BuildAuthenticationOTP(code string) ([]SendComponent, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("otp code is required")
	}
	return metapkg.BuildKodeOTPComponents(code), nil
}

// AuthenticationOTPSendLanguage returns Meta language.code for OTP sends via LanguageResolver.
func AuthenticationOTPSendLanguage() string {
	stored := template.DefaultLanguageResolver().ForStorageFromCreate(AuthenticationOTPLanguageStored)
	return template.ToMetaSendLanguage(stored)
}
