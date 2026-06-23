//go:build legacy

package campaign

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
)

func TestBuildTemplateComponents(t *testing.T) {
	email := "john@example.com"
	c := &contact.Contact{
		ID:    uuid.New(),
		Name:  "John",
		Phone: "628123",
		Email: &email,
	}

	components := buildTemplateComponents(VariableMapping{
		Body: map[string]string{
			"2": "email",
			"1": "name",
		},
	}, c)

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}
	if components[0].Type != "body" {
		t.Fatalf("expected body component")
	}
	if len(components[0].Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(components[0].Parameters))
	}
	if components[0].Parameters[0].Text != "John" {
		t.Fatalf("expected first param John, got %s", components[0].Parameters[0].Text)
	}
	if components[0].Parameters[1].Text != "john@example.com" {
		t.Fatalf("expected second param email, got %s", components[0].Parameters[1].Text)
	}
}
