package worker

import (
	"errors"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/campaign/delivery"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// FailureKind classifies Meta send outcomes.
type FailureKind int

const (
	FailurePermanent FailureKind = iota
	FailureTransient
	FailureRateLimited
)

// ClassifySendError maps Meta/client errors to retry policy.
func ClassifySendError(err error) (FailureKind, string, string) {
	if err == nil {
		return FailureTransient, "unknown", "unknown error"
	}
	if errors.Is(err, ErrRateLimited) || metaclient.IsRateLimited(err) {
		return FailureRateLimited, "rate_limited", err.Error()
	}
	var validationErr *campaign.SendValidationError
	if errors.As(err, &validationErr) {
		code := validationErr.Code
		if code == "" {
			code = campaign.ValidationCodeValidationError
		}
		return FailurePermanent, code, validationErr.Error()
	}
	if metaclient.IsInvalidToken(err) || metaclient.IsMissingResource(err) {
		return FailurePermanent, "auth_error", err.Error()
	}
	code, reason := delivery.ClassifyFailure(err.Error())
	switch code {
	case "invalid_number", "blocked", "template_error",
		campaign.ValidationCodeInvalidMediaURL,
		campaign.ValidationCodeInvalidMediaReference,
		campaign.ValidationCodeInvalidButtonParameter,
		campaign.ValidationCodeValidationError:
		return FailurePermanent, code, reason
	}
	if metaclient.IsRetryable(err) {
		return FailureTransient, code, reason
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "invalid") && strings.Contains(lower, "phone") {
		return FailurePermanent, "invalid_number", reason
	}
	return FailurePermanent, code, reason
}
