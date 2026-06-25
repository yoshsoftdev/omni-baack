package meta

import (
	"errors"
	"strings"
)

// OAuthErrorKind classifies Meta OAuth token exchange failures.
type OAuthErrorKind string

const (
	OAuthErrorInvalid OAuthErrorKind = "invalid"
	OAuthErrorExpired OAuthErrorKind = "expired"
	OAuthErrorUnknown OAuthErrorKind = "unknown"
)

// ClassifyOAuthError inspects a Meta OAuth API error body.
func ClassifyOAuthError(err error) OAuthErrorKind {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return OAuthErrorUnknown
	}
	body := strings.ToLower(apiErr.Body)
	switch {
	case strings.Contains(body, "expired"),
		strings.Contains(body, "session has expired"):
		return OAuthErrorExpired
	case strings.Contains(body, "invalid"),
		strings.Contains(body, "code"),
		strings.Contains(body, "authorization code"):
		return OAuthErrorInvalid
	default:
		return OAuthErrorUnknown
	}
}
