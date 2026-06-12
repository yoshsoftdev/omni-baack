package meta

import (
	"encoding/json"
	"testing"
)

func TestBuildKodeOTPComponents(t *testing.T) {
	components := BuildKodeOTPComponents("464646")
	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}
	if components[0].Type != "body" || components[0].Parameters[0].Text != "464646" {
		t.Fatal("unexpected body component")
	}
	if components[1].Type != "button" || components[1].SubType != "url" || components[1].Index != "0" {
		t.Fatal("unexpected button component")
	}

	data, err := json.Marshal(components)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("invalid json")
	}
}
