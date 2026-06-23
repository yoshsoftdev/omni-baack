package serviceerrors

import (
	"errors"
	"fmt"

	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Canonical service-layer errors. Handlers map these to HTTP status codes.
var (
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("forbidden")
	ErrConflict      = errors.New("conflict")
	ErrValidation    = errors.New("validation error")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// ValidationError wraps a field-level validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// NewValidation returns a validation error.
func NewValidation(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

// MapRepository converts repository errors into service errors.
func MapRepository(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, repository.ErrConflict) {
		return ErrConflict
	}
	if errors.Is(err, repository.ErrLimitExceeded) {
		return ErrQuotaExceeded
	}
	if errors.Is(err, repository.ErrInvalidInput) {
		return fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	return err
}

// IsNotFound reports whether err is a not-found service error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsForbidden reports whether err is a forbidden service error.
func IsForbidden(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsValidation reports whether err is a validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.Is(err, ErrValidation) || errors.As(err, &ve)
}
