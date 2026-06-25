package delivery

import (
	"strings"
)

// ClassifyFailure maps Meta error text to failure_code and reason.
func ClassifyFailure(raw string) (code string, reason string) {
	reason = strings.TrimSpace(raw)
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "invalid") && strings.Contains(lower, "number"),
		strings.Contains(lower, "131026"),
		strings.Contains(lower, "recipient"):
		return "invalid_number", reason
	case strings.Contains(lower, "blocked"),
		strings.Contains(lower, "131047"),
		strings.Contains(lower, "user is not valid"):
		return "blocked", reason
	case strings.Contains(lower, "rate"),
		strings.Contains(lower, "130429"),
		strings.Contains(lower, "throughput"),
		strings.Contains(lower, "limit"):
		return "rate_limited", reason
	case strings.Contains(lower, "template"),
		strings.Contains(lower, "132"),
		strings.Contains(lower, "parameter"):
		return "template_error", reason
	default:
		if reason == "" {
			reason = "unknown error"
		}
		return "unknown", reason
	}
}
