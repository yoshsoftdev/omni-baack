package serviceerrors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// Canonical service-layer errors. Handlers map these to HTTP status codes.
var (
	ErrNotFound         = errors.New("not found")
	ErrForbidden        = errors.New("forbidden")
	ErrConflict         = errors.New("conflict")
	ErrValidation       = errors.New("validation error")
	ErrQuotaExceeded    = errors.New("quota exceeded")
	ErrPaymentRequired     = errors.New("payment required")
	ErrPermissionRequired  = errors.New("permission required")
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

// NewPaymentRequired indicates broadcast send is blocked until Meta payment is configured.
func NewPaymentRequired(message string) error {
	if strings.TrimSpace(message) == "" {
		message = "Marketing campaigns require a Meta payment method."
	}
	return &PaymentRequiredError{Message: message}
}

type PaymentRequiredError struct {
	Message string
}

func (e *PaymentRequiredError) Error() string {
	return e.Message
}

func (e *PaymentRequiredError) Is(target error) bool {
	return target == ErrPaymentRequired
}

// IsPaymentRequired reports whether err is a payment-required service error.
func IsPaymentRequired(err error) bool {
	var pe *PaymentRequiredError
	return errors.Is(err, ErrPaymentRequired) || errors.As(err, &pe)
}

// NewPermissionRequired indicates Meta permissions are missing for synchronization.
func NewPermissionRequired(scope, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "Additional Meta permission is required."
	}
	return &PermissionRequiredError{Scope: scope, Message: message}
}

type PermissionRequiredError struct {
	Scope   string
	Message string
}

func (e *PermissionRequiredError) Error() string {
	return e.Message
}

func (e *PermissionRequiredError) Is(target error) bool {
	return target == ErrPermissionRequired
}

// IsPermissionRequired reports whether err is a permission-required service error.
func IsPermissionRequired(err error) bool {
	var pe *PermissionRequiredError
	return errors.Is(err, ErrPermissionRequired) || errors.As(err, &pe)
}

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
