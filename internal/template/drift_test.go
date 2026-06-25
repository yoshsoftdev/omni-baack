package template

import (
	"encoding/json"
	"testing"
)

func TestTemplateDriftChanged(t *testing.T) {
	existing := &Template{
		Name:       "welcome",
		Language:   "en_US",
		Status:     StatusApproved,
		Components: json.RawMessage(`[]`),
	}
	incoming := &Template{
		Name:       "welcome",
		Language:   "en_US",
		Status:     StatusPending,
		Components: json.RawMessage(`[]`),
	}
	if !templateDriftChanged(existing, incoming) {
		t.Fatal("expected status drift")
	}
}

func TestTemplateDriftUnchanged(t *testing.T) {
	existing := &Template{
		Name:       "welcome",
		Language:   "en_US",
		Status:     StatusApproved,
		Components: json.RawMessage(`[]`),
	}
	incoming := &Template{
		Name:       "welcome",
		Language:   "en_US",
		Status:     StatusApproved,
		Components: json.RawMessage(`[]`),
	}
	if templateDriftChanged(existing, incoming) {
		t.Fatal("expected no drift")
	}
}
