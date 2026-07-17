package message

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMediaURLServiceIssue(t *testing.T) {
	svc := NewMediaURLService("secret", "https://webhook.example.com", 5*time.Minute)
	url, err := svc.Issue(uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "https://webhook.example.com/media/eyJ") {
		t.Fatalf("url=%q", url)
	}
}

func TestMediaURLServiceRejectsLocalhost(t *testing.T) {
	svc := NewMediaURLService("secret", "http://127.0.0.1:8066", 5*time.Minute)
	_, err := svc.Issue(uuid.New(), uuid.New())
	if err == nil || !strings.Contains(err.Error(), "publicly reachable") {
		t.Fatalf("got %v", err)
	}
}
