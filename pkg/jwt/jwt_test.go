package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndParseAccess(t *testing.T) {
	manager := NewManager("access-secret", "refresh-secret", time.Minute)
	userID := uuid.New()
	teamID := uuid.New()

	token, _, err := manager.GenerateAccess(userID, teamID, "user@example.com", "owner")
	if err != nil {
		t.Fatalf("GenerateAccess() error = %v", err)
	}

	claims, err := manager.ParseAccess(token)
	if err != nil {
		t.Fatalf("ParseAccess() error = %v", err)
	}
	if claims.UserID != userID || claims.TeamID != teamID {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}
