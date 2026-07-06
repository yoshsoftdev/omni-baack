package meta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type signupStartClaims struct {
	TeamID     uuid.UUID `json:"tid"`
	UserID     uuid.UUID `json:"uid"`
	OAuthState string    `json:"st"`
	ExpiresAt  int64     `json:"exp"`
}

func issueSignupStartToken(secret string, teamID, userID uuid.UUID, oauthState string, ttl time.Duration) (string, time.Time, error) {
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, errors.New("signup start token secret is not configured")
	}
	expiresAt := time.Now().UTC().Add(ttl)
	claims := signupStartClaims{
		TeamID:     teamID,
		UserID:     userID,
		OAuthState: oauthState,
		ExpiresAt:  expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + sig, expiresAt, nil
}

func parseSignupStartToken(secret, token string) (*signupStartClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid signup start token")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return nil, fmt.Errorf("invalid signup start token signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid signup start token payload")
	}
	var claims signupStartClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("invalid signup start token claims")
	}
	if time.Now().UTC().After(time.Unix(claims.ExpiresAt, 0)) {
		return nil, fmt.Errorf("signup start token expired")
	}
	return &claims, nil
}
