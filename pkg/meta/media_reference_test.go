package meta

import (
	"encoding/json"
	"testing"
)

func TestMediaReferenceValidateDualReference(t *testing.T) {
	err := MediaReference{Link: "https://a.com/x.jpg", MediaID: "123"}.Validate()
	if err == nil {
		t.Fatal("expected dual reference error")
	}
}

func TestMediaReferenceToMediaObjectID(t *testing.T) {
	obj, err := MediaReference{MediaID: "1234567890"}.ToMediaObject()
	if err != nil {
		t.Fatal(err)
	}
	if obj.ID != "1234567890" || obj.Link != "" {
		t.Fatalf("unexpected media object: %+v", obj)
	}
}

func TestMediaObjectMarshalsIDAsJSONNumber(t *testing.T) {
	raw, err := json.Marshal(MediaObject{ID: "9876543210"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"id":9876543210}` {
		t.Fatalf("expected numeric id JSON, got %s", raw)
	}
}

func TestBuildImageHeaderMediaID(t *testing.T) {
	components := []byte(`[{"type":"HEADER","format":"IMAGE"}]`)
	payload, err := BuildTemplatePayload(components, VariableMappingInput{
		HeaderMedia: MediaReference{MediaID: "123456"},
	}, func(_, _ string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if payload.Components[0].Parameters[0].Image.ID != "123456" {
		t.Fatalf("expected media id payload, got %+v", payload.Components[0].Parameters[0].Image)
	}
}

func TestBuildImageHeaderDualReferenceFails(t *testing.T) {
	components := []byte(`[{"type":"HEADER","format":"IMAGE"}]`)
	_, err := BuildTemplatePayload(components, VariableMappingInput{
		HeaderMedia: MediaReference{Link: "https://a.com/x.jpg", MediaID: "1"},
	}, func(_, _ string) string { return "" })
	if err == nil {
		t.Fatal("expected build failure for dual media reference")
	}
}
