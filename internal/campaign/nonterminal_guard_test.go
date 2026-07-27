package campaign_test

import (
	"os"
	"strings"
	"testing"
)

func TestCountNonTerminalIncludesSentDelivered(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	needle := "status NOT IN ('sent', 'delivered', 'read', 'failed', 'cancelled', 'skipped')"
	if !strings.Contains(body, needle) {
		t.Fatalf("CountNonTerminalRecipients must treat sent/delivered as terminal for campaign completion")
	}
}
