package meta

import "testing"

func TestMediaReferenceValidateDualReference(t *testing.T) {
	err := MediaReference{Link: "https://a.com/x.jpg", MediaID: "123"}.Validate()
	if err == nil {
		t.Fatal("expected dual reference error")
	}
}

func TestMediaReferenceToMediaObjectID(t *testing.T) {
	obj, err := MediaReference{MediaID: "meta-media-99"}.ToMediaObject()
	if err != nil {
		t.Fatal(err)
	}
	if obj.ID != "meta-media-99" || obj.Link != "" {
		t.Fatalf("unexpected media object: %+v", obj)
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
