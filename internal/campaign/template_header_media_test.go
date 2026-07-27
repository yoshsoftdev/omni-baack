package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/media"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

func TestExtractTemplateHeaderMediaFromTemplateUsesMetadataFallback(t *testing.T) {
	storageID := "11111111-1111-1111-1111-111111111111"
	tpl := &template.Template{
		Components: json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"]}},{"type":"BODY","text":"Hi"}]`),
		Metadata:   template.EmptyMetadata().WithStorageObjectIDs([]string{storageID}),
	}
	mediaMeta, ok := ExtractTemplateHeaderMediaFromTemplate(tpl)
	if !ok {
		t.Fatal("expected header media from metadata")
	}
	if mediaMeta.StorageObjectID != storageID {
		t.Fatalf("storage = %q", mediaMeta.StorageObjectID)
	}
	if !TemplateHasStorageBackedHeaderMedia(tpl) {
		t.Fatal("expected storage-backed header via metadata")
	}
}

func TestExtractTemplateHeaderMedia(t *testing.T) {
	components := json.RawMessage(`[
		{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"],"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},
		{"type":"BODY","text":"Hi {{1}}"}
	]`)

	mediaMeta, ok := ExtractTemplateHeaderMedia(components)
	if !ok {
		t.Fatal("expected header media")
	}
	if mediaMeta.Handle != "4::abc" {
		t.Fatalf("handle = %q", mediaMeta.Handle)
	}
	if mediaMeta.StorageObjectID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("storage object id = %q", mediaMeta.StorageObjectID)
	}
}

func TestApplyTemplateHeaderMediaDefaultsDoesNotUseHandleAsMediaID(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::meta-handle-1"]}}]`)
	tpl := &template.Template{Components: components}

	mapping := ApplyTemplateHeaderMediaDefaults(VariableMapping{}, tpl)
	if headerMappingHasCloudMediaID(mapping.Header) || headerMappingHasMediaLink(mapping.Header) {
		t.Fatalf("header_handle must not become send media, got %+v", mapping.Header)
	}
}

func TestApplyTemplateHeaderMediaDefaultsKeepsCampaignOverride(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::meta-handle-1"]}}]`)
	tpl := &template.Template{Components: components}

	mapping := ApplyTemplateHeaderMediaDefaults(VariableMapping{
		Header: &HeaderMapping{MediaLink: "https://cdn.example.com/banner.jpg"},
	}, tpl)
	if mapping.Header.MediaLink != "https://cdn.example.com/banner.jpg" {
		t.Fatalf("expected campaign media link to win, got %+v", mapping.Header)
	}
}

func TestValidateTemplateSendRequiresHeaderMediaWithoutStorage(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::meta-handle-1"]}},{"type":"BODY","text":"Hi"}]`)
	tpl := &template.Template{Components: components}

	err := ValidateTemplateSend(tpl, VariableMapping{}, nil)
	if err == nil {
		t.Fatal("expected validation error when only template header_handle is present")
	}
	valErr, ok := err.(*SendValidationError)
	if !ok {
		t.Fatalf("expected SendValidationError, got %T", err)
	}
	if valErr.Code != ValidationCodeMissingVariable {
		t.Fatalf("expected missing_variable, got %s", valErr.Code)
	}
}

func TestValidateTemplateSendAllowsStorageBackedHeaderDefault(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"storage_object_id":["11111111-1111-1111-1111-111111111111"]}},{"type":"BODY","text":"Hi"}]`)
	tpl := &template.Template{Components: components}

	if err := ValidateTemplateSend(tpl, VariableMapping{}, nil); err != nil {
		t.Fatalf("expected storage-backed header to pass pre-send validation: %v", err)
	}
}

func TestIsWhatsAppCloudMediaID(t *testing.T) {
	if !IsWhatsAppCloudMediaID("1234567890") {
		t.Fatal("expected numeric media id")
	}
	if IsWhatsAppCloudMediaID("4::abc") {
		t.Fatal("header_handle must not be treated as cloud media id")
	}
	if IsWhatsAppCloudMediaID("meta-handle-1") {
		t.Fatal("non-numeric id must be rejected")
	}
}

func TestResolveHeaderMediaForSendClearsInvalidMediaID(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"header_handle":["4::abc"]}}]`)
	tpl := &template.Template{Components: components}

	mapping, err := ResolveHeaderMediaForSend(
		t.Context(),
		VariableMapping{Header: &HeaderMapping{MediaID: "4::abc"}},
		tpl,
		ResolveHeaderMediaInput{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Header == nil || mapping.Header.MediaID != "" {
		t.Fatalf("expected invalid media_id cleared, got %+v", mapping.Header)
	}
}

func TestResolveHeaderMediaForSendDeprecatedMediaLink(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE"}]`)
	tpl := &template.Template{Components: components}
	metrics := &media.MemoryMetrics{}

	mapping, err := ResolveHeaderMediaForSend(
		t.Context(),
		VariableMapping{Header: &HeaderMapping{MediaLink: "https://cdn.example.com/x.jpg"}},
		tpl,
		ResolveHeaderMediaInput{Metrics: metrics},
	)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Header.MediaLink != "https://cdn.example.com/x.jpg" {
		t.Fatalf("expected media_link preserved, got %+v", mapping.Header)
	}
	if metrics.DeprecatedLink != 1 {
		t.Fatalf("expected deprecated link metric, got %d", metrics.DeprecatedLink)
	}
}

type stubMediaManager struct {
	ref *media.MediaReference
	err error
	mu  sync.Mutex
	n   int
}

func (s *stubMediaManager) EnsureMedia(context.Context, media.EnsureInput) (*media.MediaReference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	if s.err != nil {
		return nil, s.err
	}
	return s.ref, nil
}
func (s *stubMediaManager) RegisterAsset(context.Context, media.MediaAsset) error { return nil }
func (s *stubMediaManager) FindAsset(context.Context, uuid.UUID, uuid.UUID) (*media.MediaAsset, error) {
	return nil, media.ErrAssetNotFound
}

func TestResolveHeaderMediaForSendPrefersEnsureMediaOverLink(t *testing.T) {
	assetID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"storage_object_id":["11111111-1111-1111-1111-111111111111"]}}]`)
	tpl := &template.Template{Components: components}
	mgr := &stubMediaManager{ref: &media.MediaReference{
		AssetID: assetID, ProviderMediaID: "999888", Source: media.MediaSourceCacheMissUpload, MimeType: "image/jpeg",
	}}
	metrics := &media.MemoryMetrics{}

	mapping, err := ResolveHeaderMediaForSend(
		t.Context(),
		VariableMapping{Header: &HeaderMapping{MediaLink: "https://cdn.example.com/ignored.jpg"}},
		tpl,
		ResolveHeaderMediaInput{
			TeamID:        uuid.New(),
			PhoneNumberID: "pn",
			AccessToken:   "tok",
			Manager:       mgr,
			Metrics:       metrics,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Header.MediaID != "999888" {
		t.Fatalf("expected EnsureMedia id, got %+v", mapping.Header)
	}
	if mapping.Header.MediaLink != "" {
		t.Fatalf("media_link must be cleared when EnsureMedia succeeds, got %q", mapping.Header.MediaLink)
	}
	if metrics.DeprecatedLink != 0 {
		t.Fatal("deprecated link must not be used when storage path wins")
	}
	if mgr.n != 1 {
		t.Fatalf("EnsureMedia calls=%d", mgr.n)
	}
}

func TestResolveHeaderMediaForSendEnsureFailure(t *testing.T) {
	components := json.RawMessage(`[{"type":"HEADER","format":"IMAGE","example":{"storage_object_id":["11111111-1111-1111-1111-111111111111"]}}]`)
	tpl := &template.Template{Components: components}
	mgr := &stubMediaManager{err: errors.New("failed to upload media to Meta")}

	_, err := ResolveHeaderMediaForSend(
		t.Context(),
		VariableMapping{},
		tpl,
		ResolveHeaderMediaInput{
			TeamID: uuid.New(), PhoneNumberID: "pn", AccessToken: "tok", Manager: mgr,
		},
	)
	if err == nil {
		t.Fatal("expected EnsureMedia error to propagate")
	}
}
