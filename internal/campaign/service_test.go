package campaign

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateSendMode(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	if err := validateSendMode(SendNow, nil); err != nil {
		t.Fatalf("SendNow: %v", err)
	}
	if err := validateSendMode(SendLater, &future); err != nil {
		t.Fatalf("SendLater future: %v", err)
	}
	if err := validateSendMode(SendLater, nil); err == nil {
		t.Fatal("expected error for missing scheduled_at")
	}
	past := time.Now().UTC().Add(-time.Hour)
	if err := validateSendMode(SendLater, &past); err == nil {
		t.Fatal("expected error for past scheduled_at")
	}
}

func TestValidateAudienceSource(t *testing.T) {
	if err := validateAudienceSource(AudienceInput{SourceType: AudienceTag, SourceID: uuid.New()}); err != nil {
		t.Fatalf("valid tag audience: %v", err)
	}
	if err := validateAudienceSource(AudienceInput{SourceType: AudienceSegment, SourceID: uuid.New()}); err == nil {
		t.Fatal("expected error for segment audience")
	}
	if err := validateAudienceSource(AudienceInput{SourceType: "invalid"}); err == nil {
		t.Fatal("expected error for invalid source type")
	}
}
