package phone

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidIndonesiaFormat = errors.New("phone must be a valid Indonesian number in +62 format")

	indonesiaE164 = regexp.MustCompile(`^\+62[1-9][0-9]{7,11}$`)
	digitsOnly    = regexp.MustCompile(`^[0-9]+$`)
)

// ValidateIndonesiaE164 checks +62 Indonesian mobile/landline format (e.g. +628123456789).
func ValidateIndonesiaE164(raw string) (string, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	if !indonesiaE164.MatchString(cleaned) {
		return "", ErrInvalidIndonesiaFormat
	}
	return cleaned, nil
}

// ToMetaRecipient strips the leading + for Meta WhatsApp API "to" field.
func ToMetaRecipient(e164 string) string {
	return strings.TrimPrefix(strings.TrimSpace(e164), "+")
}

// ValidateNumericCode checks OTP code contains digits only.
func ValidateNumericCode(code string) error {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" || !digitsOnly.MatchString(trimmed) {
		return errors.New("code must contain digits only")
	}
	if len(trimmed) > 20 {
		return errors.New("code is too long")
	}
	return nil
}
