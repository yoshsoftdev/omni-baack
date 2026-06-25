package meta

import "encoding/json"

// TemplateDefinitionComponent is a parsed Meta template component from synced JSON.
type TemplateDefinitionComponent struct {
	Type    string                   `json:"type"`
	Format  string                   `json:"format,omitempty"`
	Text    string                   `json:"text,omitempty"`
	Buttons []TemplateDefinitionButton `json:"buttons,omitempty"`
}

// TemplateDefinitionButton is a button inside a BUTTONS component.
type TemplateDefinitionButton struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	URL         string `json:"url,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
	OTPType     string `json:"otp_type,omitempty"`
}

// VariableMappingInput carries campaign variable mapping for template rendering.
type VariableMappingInput struct {
	HeaderText  map[string]string
	HeaderMedia MediaReference
	Body        map[string]string
	Buttons     map[string]ButtonMappingInput
}

// ButtonMappingInput maps button-level variables for send-time rendering.
type ButtonMappingInput struct {
	URLVars  map[string]string
	CopyCode string
}

// MetaTemplatePayload is the send-time template component list for Meta Cloud API.
type MetaTemplatePayload struct {
	Components []TemplateComponent
}

// ResolveFunc resolves a mapping source for a variable index within a component scope.
type ResolveFunc func(index, source string) string

// ParseTemplateDefinitionComponents unmarshals synced template components JSON.
func ParseTemplateDefinitionComponents(raw json.RawMessage) ([]TemplateDefinitionComponent, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var items []TemplateDefinitionComponent
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}
