package meta

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{\{(\d+)\}\}`)

// BuildTemplatePayload renders Meta send components from a synced template definition.
func BuildTemplatePayload(
	componentsJSON json.RawMessage,
	mapping VariableMappingInput,
	resolve ResolveFunc,
) (*MetaTemplatePayload, error) {
	defs, err := ParseTemplateDefinitionComponents(componentsJSON)
	if err != nil {
		return nil, fmt.Errorf("parse template components: %w", err)
	}

	out := make([]TemplateComponent, 0, len(defs))
	for _, def := range defs {
		switch strings.ToUpper(strings.TrimSpace(def.Type)) {
		case "HEADER":
			component, err := buildHeaderComponent(def, mapping, resolve)
			if err != nil {
				return nil, err
			}
			if component != nil {
				out = append(out, *component)
			}
		case "BODY":
			component, err := buildBodyComponent(def, mapping, resolve)
			if err != nil {
				return nil, err
			}
			if component != nil {
				out = append(out, *component)
			}
		case "FOOTER":
			// Footer is static in the approved template; no send-time component required.
			continue
		case "BUTTONS":
			buttonComponents, err := buildButtonComponents(def, mapping, resolve)
			if err != nil {
				return nil, err
			}
			out = append(out, buttonComponents...)
		}
	}

	return &MetaTemplatePayload{Components: out}, nil
}

func buildHeaderComponent(def TemplateDefinitionComponent, mapping VariableMappingInput, resolve ResolveFunc) (*TemplateComponent, error) {
	format := strings.ToUpper(strings.TrimSpace(def.Format))
	if format == "" {
		format = "TEXT"
	}

	switch format {
	case "TEXT":
		indices := extractPlaceholderIndices(def.Text)
		if len(indices) == 0 {
			return nil, nil
		}
		params, err := textParameters("header", indices, mapping.HeaderText, resolve)
		if err != nil {
			return nil, err
		}
		return &TemplateComponent{Type: "header", Parameters: params}, nil
	case "IMAGE", "VIDEO", "DOCUMENT":
		mediaObj, err := mapping.HeaderMedia.ToMediaObject()
		if err != nil {
			if strings.TrimSpace(mapping.HeaderMedia.Link) == "" && strings.TrimSpace(mapping.HeaderMedia.MediaID) == "" {
				return nil, nil
			}
			return nil, err
		}
		param := TemplateParameter{Type: strings.ToLower(format)}
		switch format {
		case "IMAGE":
			param.Image = &mediaObj
		case "VIDEO":
			param.Video = &mediaObj
		case "DOCUMENT":
			param.Document = &mediaObj
		}
		return &TemplateComponent{
			Type:       "header",
			Parameters: []TemplateParameter{param},
		}, nil
	default:
		return nil, nil
	}
}

func buildBodyComponent(def TemplateDefinitionComponent, mapping VariableMappingInput, resolve ResolveFunc) (*TemplateComponent, error) {
	indices := extractPlaceholderIndices(def.Text)
	if len(indices) == 0 {
		return nil, nil
	}
	params, err := textParameters("body", indices, mapping.Body, resolve)
	if err != nil {
		return nil, err
	}
	return &TemplateComponent{Type: "body", Parameters: params}, nil
}

func buildButtonComponents(def TemplateDefinitionComponent, mapping VariableMappingInput, resolve ResolveFunc) ([]TemplateComponent, error) {
	out := make([]TemplateComponent, 0, len(def.Buttons))
	for i, btn := range def.Buttons {
		index := strconv.Itoa(i)
		btnMapping := mapping.Buttons[index]
		btnType := normalizeButtonType(btn.Type, btn.OTPType)

		switch btnType {
		case "QUICK_REPLY":
			continue
		case "URL":
			indices := extractPlaceholderIndices(btn.URL)
			if len(indices) == 0 {
				continue
			}
			params, err := textParameters("button:"+index, indices, btnMapping.URLVars, resolve)
			if err != nil {
				return nil, err
			}
			out = append(out, TemplateComponent{
				Type:       "button",
				SubType:    "url",
				Index:      index,
				Parameters: params,
			})
		case "COPY_CODE":
			code := strings.TrimSpace(btnMapping.CopyCode)
			if code == "" {
				continue
			}
			out = append(out, TemplateComponent{
				Type:    "button",
				SubType: "copy_code",
				Index:   index,
				Parameters: []TemplateParameter{{
					Type:       "coupon_code",
					CouponCode: code,
				}},
			})
		}
	}
	return out, nil
}

func textParameters(scope string, indices []int, mapping map[string]string, resolve ResolveFunc) ([]TemplateParameter, error) {
	if len(indices) == 0 {
		return nil, nil
	}
	params := make([]TemplateParameter, 0, len(indices))
	for _, n := range indices {
		key := strconv.Itoa(n)
		source, ok := mapping[key]
		if !ok {
			return nil, fmt.Errorf("%s: missing mapping for variable {{%s}}", scope, key)
		}
		value := strings.TrimSpace(resolve(key, source))
		if value == "" {
			return nil, fmt.Errorf("%s: empty value for variable {{%s}}", scope, key)
		}
		params = append(params, TemplateParameter{Type: "text", Text: value})
	}
	return params, nil
}

func extractPlaceholderIndices(text string) []int {
	matches := placeholderPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		seen[n] = struct{}{}
	}
	indices := make([]int, 0, len(seen))
	for n := range seen {
		indices = append(indices, n)
	}
	sort.Ints(indices)
	return indices
}

func normalizeButtonType(rawType, otpType string) string {
	raw := strings.ToUpper(strings.TrimSpace(rawType))
	if raw == "OTP" {
		if strings.EqualFold(strings.TrimSpace(otpType), "COPY_CODE") {
			return "COPY_CODE"
		}
		return raw
	}
	return raw
}

// RequiredTemplateVariables returns variable indices and media/button requirements from a template.
func RequiredTemplateVariables(componentsJSON json.RawMessage) (RequiredVariables, error) {
	defs, err := ParseTemplateDefinitionComponents(componentsJSON)
	if err != nil {
		return RequiredVariables{}, err
	}

	req := RequiredVariables{
		HeaderText: make(map[string]struct{}),
		Body:       make(map[string]struct{}),
		ButtonURL:  make(map[string]map[string]struct{}),
		ButtonCopy: make(map[string]struct{}),
	}
	for _, def := range defs {
		switch strings.ToUpper(strings.TrimSpace(def.Type)) {
		case "HEADER":
			format := strings.ToUpper(strings.TrimSpace(def.Format))
			if format == "" {
				format = "TEXT"
			}
			switch format {
			case "TEXT":
				for _, idx := range extractPlaceholderIndices(def.Text) {
					req.HeaderText[strconv.Itoa(idx)] = struct{}{}
				}
			case "IMAGE", "VIDEO", "DOCUMENT":
				req.HeaderMedia = true
			}
		case "BODY":
			for _, idx := range extractPlaceholderIndices(def.Text) {
				req.Body[strconv.Itoa(idx)] = struct{}{}
			}
		case "BUTTONS":
			for i, btn := range def.Buttons {
				index := strconv.Itoa(i)
				switch normalizeButtonType(btn.Type, btn.OTPType) {
				case "URL":
					urlIndices := extractPlaceholderIndices(btn.URL)
					if len(urlIndices) == 0 {
						continue
					}
					req.ButtonURL[index] = make(map[string]struct{}, len(urlIndices))
					for _, idx := range urlIndices {
						req.ButtonURL[index][strconv.Itoa(idx)] = struct{}{}
					}
				case "COPY_CODE":
					req.ButtonCopy[index] = struct{}{}
				}
			}
		}
	}
	return req, nil
}

// RequiredVariables describes send-time values required by a template definition.
type RequiredVariables struct {
	HeaderText map[string]struct{}
	HeaderMedia bool
	Body       map[string]struct{}
	ButtonURL  map[string]map[string]struct{}
	ButtonCopy map[string]struct{}
}
