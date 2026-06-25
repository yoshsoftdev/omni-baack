package template_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestMapFromMetaNormalizesStatusAndLanguage(t *testing.T) {
	channelID := uuid.New()
	teamID := uuid.New()
	preview := "Hello {{1}}"
	raw := []byte(`[{"type":"BODY","text":"Hello {{1}}"},{"type":"BUTTONS","buttons":[{"type":"QUICK_REPLY","text":"OK"}]}]`)

	tmpl := template.MapFromMeta(channelID, teamID, "waba-1", metapkg.MessageTemplate{
		ID:         "meta-1",
		Name:       "welcome",
		Language:   "en",
		Status:     "approved",
		Category:   "MARKETING",
		Components: raw,
	}, time.Now().UTC())

	if tmpl.Status != template.StatusApproved {
		t.Fatalf("status = %s", tmpl.Status)
	}
	if tmpl.Language != "en_EN" {
		t.Fatalf("language = %s", tmpl.Language)
	}
	if tmpl.VariableCount != 1 {
		t.Fatalf("variable count = %d", tmpl.VariableCount)
	}
	if tmpl.Preview == nil || *tmpl.Preview != preview {
		t.Fatalf("preview = %v", tmpl.Preview)
	}
}
