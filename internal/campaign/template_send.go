package campaign

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/contact"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// HeaderMapping configures header variables and media for template send.
type HeaderMapping struct {
	Text      map[string]string `json:"text,omitempty"`
	MediaLink string            `json:"media_link,omitempty"`
	MediaID   string            `json:"media_id,omitempty"`
}

// MediaReference returns the header media reference from mapping fields.
func (h *HeaderMapping) MediaReference() metaclient.MediaReference {
	if h == nil {
		return metaclient.MediaReference{}
	}
	return metaclient.MediaReference{
		Link:    h.MediaLink,
		MediaID: h.MediaID,
	}
}

// ButtonMapping configures button variables for template send.
type ButtonMapping struct {
	URL      map[string]string `json:"url,omitempty"`
	CopyCode string            `json:"copy_code,omitempty"`
}

// VariableMapping maps template variable indices to contact fields or literals.
type VariableMapping struct {
	Header  *HeaderMapping            `json:"header,omitempty"`
	Body    map[string]string         `json:"body,omitempty"`
	Buttons map[string]ButtonMapping  `json:"buttons,omitempty"`
}

// ParseVariableMapping decodes campaign variable_mapping JSON.
func ParseVariableMapping(raw json.RawMessage) (VariableMapping, error) {
	if len(raw) == 0 {
		return VariableMapping{}, nil
	}
	var mapping VariableMapping
	if err := json.Unmarshal(raw, &mapping); err != nil {
		return VariableMapping{}, fmt.Errorf("invalid variable_mapping: %w", err)
	}
	return mapping, nil
}

// ToMetaMapping converts campaign mapping to the Meta builder input model.
func (m VariableMapping) ToMetaMapping() metaclient.VariableMappingInput {
	out := metaclient.VariableMappingInput{
		Body:    m.Body,
		Buttons: make(map[string]metaclient.ButtonMappingInput, len(m.Buttons)),
	}
	if m.Header != nil {
		out.HeaderText = m.Header.Text
		out.HeaderMedia = m.Header.MediaReference()
	}
	for index, btn := range m.Buttons {
		out.Buttons[index] = metaclient.ButtonMappingInput{
			URLVars:  btn.URL,
			CopyCode: btn.CopyCode,
		}
	}
	return out
}

// NewVariableResolver resolves mapping sources using contact fields and sample values.
func NewVariableResolver(contact *contact.Contact, sampleValues map[string]string) metaclient.ResolveFunc {
	return func(index, source string) string {
		return ResolveMappedValue(source, contact, sampleValues, index)
	}
}

// ResolveMappedValue resolves a mapping source for a variable index.
func ResolveMappedValue(source string, c *contact.Contact, sampleValues map[string]string, index string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if c != nil {
		if value := resolveContactField(c, source); value != "" {
			return value
		}
	}
	if sampleValues != nil {
		if value := strings.TrimSpace(sampleValues[index]); value != "" {
			return value
		}
		if value := strings.TrimSpace(sampleValues[source]); value != "" {
			return value
		}
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return source
	}
	return ""
}

func resolveContactField(c *contact.Contact, field string) string {
	if c == nil {
		return ""
	}
	switch field {
	case "name", "customer_name":
		return c.CustomerName
	case "contact_name":
		return c.ContactName
	case "phone":
		return c.Phone
	case "email":
		if c.Email != nil {
			return *c.Email
		}
	case "notes":
		if c.Notes != nil {
			return *c.Notes
		}
	}
	return ""
}

// ParseSampleValues decodes campaign sample_values JSON into a string map.
func ParseSampleValues(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

// BuildTemplateComponents renders Meta send components using the rich template builder.
func BuildTemplateComponents(
	componentsJSON json.RawMessage,
	mapping VariableMapping,
	contact *contact.Contact,
	sampleValues map[string]string,
) ([]metaclient.TemplateComponent, error) {
	payload, err := metaclient.BuildTemplatePayload(
		componentsJSON,
		mapping.ToMetaMapping(),
		NewVariableResolver(contact, sampleValues),
	)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, nil
	}
	return payload.Components, nil
}
