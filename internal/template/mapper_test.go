package template

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestToMetaCreateRequestStripsInternalFields(t *testing.T) {
	t.Parallel()
	domain := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hi"}
	]`)
	req, err := ToMetaCreateRequest("promo_x", "en_US", "MARKETING", domain)
	if err != nil {
		t.Fatalf("ToMetaCreateRequest: %v", err)
	}
	if req.Name != "promo_x" || req.Language != "en_US" || req.Category != "MARKETING" {
		t.Fatalf("request fields = %+v", req)
	}
	var items []map[string]any
	if err := json.Unmarshal(req.Components, &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	example, _ := items[0]["example"].(map[string]any)
	if _, ok := example["storage_object_id"]; ok {
		t.Fatal("storage_object_id must not appear in Meta create payload")
	}
	handles, _ := example["header_handle"].([]any)
	if len(handles) != 1 || handles[0] != "4::abc" {
		t.Fatalf("header_handle = %#v", handles)
	}
}

func TestMapFromMetaCreateKeepsDomainMetadata(t *testing.T) {
	t.Parallel()
	channelID := uuid.New()
	teamID := uuid.New()
	domain := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hello {{1}}"}
	]`)
	tmpl := MapFromMetaCreate(
		channelID, teamID, "waba-1", "welcome", "en_US", "MARKETING",
		domain,
		metapkg.CreateTemplateResponse{ID: "meta-9", Status: "PENDING", Category: "UTILITY"},
		time.Unix(1700000000, 0).UTC(),
	)
	if tmpl.MetaTemplateID != "meta-9" {
		t.Fatalf("meta id = %s", tmpl.MetaTemplateID)
	}
	if tmpl.Status != StatusPending {
		t.Fatalf("status = %s", tmpl.Status)
	}
	if tmpl.Category == nil || *tmpl.Category != "UTILITY" {
		t.Fatalf("category = %#v", tmpl.Category)
	}
	ids := tmpl.Metadata.StorageObjectIDs()
	if len(ids) != 1 || ids[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("metadata storage ids = %#v", ids)
	}
	if tmpl.VariableCount != 1 {
		t.Fatalf("variable_count = %d", tmpl.VariableCount)
	}
	if !strings.Contains(string(tmpl.Components), "storage_object_id") {
		t.Fatal("domain components should retain storage_object_id")
	}
}

func TestToMetaSendLanguage(t *testing.T) {
	t.Parallel()
	if got := ToMetaSendLanguage("en_US"); got != "en_US" {
		t.Fatalf("got %s", got)
	}
	if got := ToMetaSendLanguage(""); got != "en_US" {
		t.Fatalf("empty → %s", got)
	}
	if got := ToMetaSendLanguage(" id "); got != "id" {
		t.Fatalf("trim → %s", got)
	}
	if got := ToMetaSendLanguage("en_EN"); got != "en_US" {
		t.Fatalf("en_EN artifact → %s", got)
	}
}

func TestComponentsForMetaAPI(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"],"storage_object_id":["x"]}},
		{"type":"BODY","text":"Hi"}
	]`)
	out, err := ComponentsForMetaAPI(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Contains(string(out), "storage_object_id") {
		t.Fatal("storage_object_id must be stripped")
	}
}
