package campaign

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
)

// ParseRecipientVariables decodes campaign_recipients.variables JSONB.
// Empty/null payloads become an empty map. Missing keys are not invented.
func ParseRecipientVariables(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return map[string]string{}, nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("invalid recipient variables snapshot: %w", err)
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}

// MaterializeMappingFromSnapshot replaces mapping field sources with frozen snapshot values.
// Campaign-level media / copy_code mappings are preserved.
// Missing required snapshot keys return an error — Contact fallback is forbidden.
func MaterializeMappingFromSnapshot(mapping VariableMapping, variables map[string]string) (VariableMapping, error) {
	if variables == nil {
		variables = map[string]string{}
	}
	out := VariableMapping{
		Body:    map[string]string{},
		Buttons: map[string]ButtonMapping{},
	}
	if mapping.Header != nil {
		out.Header = &HeaderMapping{
			MediaLink: mapping.Header.MediaLink,
			MediaID:   mapping.Header.MediaID,
		}
		if len(mapping.Header.Text) > 0 {
			out.Header.Text = make(map[string]string, len(mapping.Header.Text))
			for index := range mapping.Header.Text {
				val, err := requireSnapshotValue(variables, "header."+index)
				if err != nil {
					return VariableMapping{}, err
				}
				out.Header.Text[index] = val
			}
		}
	}
	for index := range mapping.Body {
		val, err := requireSnapshotValue(variables, "body."+index)
		if err != nil {
			return VariableMapping{}, err
		}
		out.Body[index] = val
	}
	for btnIndex, btn := range mapping.Buttons {
		copied := ButtonMapping{CopyCode: btn.CopyCode}
		if len(btn.URL) > 0 {
			copied.URL = make(map[string]string, len(btn.URL))
			for varIndex := range btn.URL {
				val, err := requireSnapshotValue(variables, "button."+btnIndex+"."+varIndex)
				if err != nil {
					return VariableMapping{}, err
				}
				copied.URL[varIndex] = val
			}
		}
		out.Buttons[btnIndex] = copied
	}
	return out, nil
}

func requireSnapshotValue(variables map[string]string, key string) (string, error) {
	val := strings.TrimSpace(variables[key])
	if val == "" {
		return "", fmt.Errorf("missing snapshot variable %q", key)
	}
	return val, nil
}

// BuildTemplateComponents renders send components via Template Builder from components JSON.
// Prefer BuildTemplateComponentsForTemplate when a domain Template is available.
func BuildTemplateComponents(
	componentsJSON json.RawMessage,
	mapping VariableMapping,
) ([]builder.SendComponent, error) {
	return BuildTemplateComponentsForTemplate(&template.Template{Components: componentsJSON}, mapping)
}

// BuildTemplateComponentsForTemplate renders send components for a domain Template.
func BuildTemplateComponentsForTemplate(
	tmpl *template.Template,
	mapping VariableMapping,
) ([]builder.SendComponent, error) {
	return builder.Build(tmpl, mapping.ToBuilderVariables())
}

// BuildTemplateComponentsFromSnapshot materializes mapping from recipient variables then renders.
func BuildTemplateComponentsFromSnapshot(
	componentsJSON json.RawMessage,
	mapping VariableMapping,
	variables map[string]string,
) ([]builder.SendComponent, error) {
	materialized, err := MaterializeMappingFromSnapshot(mapping, variables)
	if err != nil {
		return nil, err
	}
	return BuildTemplateComponents(componentsJSON, materialized)
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

// ParseSampleValues decodes campaign sample_values JSON into a string map.
// Sample values are for create-time preview only; workers must not use them as send fallback.
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

// HeaderMapping configures header variables and media for template send.
type HeaderMapping struct {
	Text      map[string]string `json:"text,omitempty"`
	MediaLink string            `json:"media_link,omitempty"`
	MediaID   string            `json:"media_id,omitempty"`
}

// MediaReference returns the header media reference for Template Builder.
func (h *HeaderMapping) MediaReference() builder.MediaRef {
	if h == nil {
		return builder.MediaRef{}
	}
	return builder.MediaRef{
		Link:    h.MediaLink,
		MediaID: h.MediaID,
	}
}

// ButtonMapping configures button variables for template send.
type ButtonMapping struct {
	URL      map[string]string `json:"url,omitempty"`
	CopyCode string            `json:"copy_code,omitempty"`
}

// VariableMapping maps template variable indices to contact fields or literals at create time.
// At send time, MaterializeMappingFromSnapshot replaces field sources with frozen values.
type VariableMapping struct {
	Header  *HeaderMapping           `json:"header,omitempty"`
	Body    map[string]string        `json:"body,omitempty"`
	Buttons map[string]ButtonMapping `json:"buttons,omitempty"`
}

// ToBuilderVariables converts campaign mapping to Template Builder input.
func (m VariableMapping) ToBuilderVariables() builder.Variables {
	out := builder.Variables{
		Body:    m.Body,
		Buttons: make(map[string]builder.ButtonVars, len(m.Buttons)),
	}
	if m.Header != nil {
		out.HeaderText = m.Header.Text
		out.HeaderMedia = m.Header.MediaReference()
	}
	for index, btn := range m.Buttons {
		out.Buttons[index] = builder.ButtonVars{
			URLVars:  btn.URL,
			CopyCode: btn.CopyCode,
		}
	}
	return out
}
