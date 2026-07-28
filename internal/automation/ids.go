package automation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func NewEndpointID() string {
	return "ept_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

func NewAPIKeyID() string {
	return "key_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

func NewExecutionID() string {
	return "exec_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

func NewWebhookDeliveryID() string {
	return "whd_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

func NewForwardWebhookID() string {
	return "whk_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

func NewForwardDeliveryID() string {
	return "fwd_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

func GenerateAPIKeySecret() (secret string, prefix string, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}
	secret = "oc_auto_" + hex.EncodeToString(buf)
	prefix = secret
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	hash = HashAPIKey(secret)
	return secret, prefix, hash, nil
}

func HashAPIKey(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func Slugify(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = out[:64]
		out = strings.Trim(out, "-")
	}
	return out
}

func IsValidSlug(slug string) bool {
	if len(slug) < 2 || len(slug) > 64 {
		return false
	}
	for i, r := range slug {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r == '-' && i > 0 && i < len(slug)-1)
		if !ok {
			return false
		}
	}
	return !strings.Contains(slug, "--")
}
