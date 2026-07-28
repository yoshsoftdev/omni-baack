package campaign

import (
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
)

// ApplyTemplateButtonDefaults fills missing URL / copy_code button mappings from body
// placeholders with the same index. Authentication OTP templates store "Salin Kode" as a
// URL button with {{n}} in the WhatsApp OTP URL — callers often only map body vars.
func ApplyTemplateButtonDefaults(
	tpl *template.Template,
	mapping VariableMapping,
	variables map[string]string,
) (VariableMapping, map[string]string) {
	if tpl == nil {
		return mapping, variables
	}
	if variables == nil {
		variables = map[string]string{}
	}
	required, err := builder.RequiredFrom(tpl)
	if err != nil {
		return mapping, variables
	}
	if mapping.Buttons == nil {
		mapping.Buttons = map[string]ButtonMapping{}
	}

	for btnIndex, urlVars := range required.ButtonURL {
		btn := mapping.Buttons[btnIndex]
		if btn.URL == nil {
			btn.URL = map[string]string{}
		}
		changed := false
		for varIndex := range urlVars {
			if strings.TrimSpace(btn.URL[varIndex]) != "" {
				continue
			}
			bodySource := strings.TrimSpace(mapping.Body[varIndex])
			bodyVal := strings.TrimSpace(variables["body."+varIndex])
			if bodySource == "" && bodyVal == "" {
				continue
			}
			if bodySource == "" {
				bodySource = bodyVal
			}
			btn.URL[varIndex] = bodySource
			if bodyVal != "" {
				variables["button."+btnIndex+"."+varIndex] = bodyVal
			}
			changed = true
		}
		if changed {
			mapping.Buttons[btnIndex] = btn
		}
	}

	for btnIndex := range required.ButtonCopy {
		btn := mapping.Buttons[btnIndex]
		if strings.TrimSpace(btn.CopyCode) != "" {
			continue
		}
		if val := strings.TrimSpace(variables["body.1"]); val != "" {
			btn.CopyCode = val
			mapping.Buttons[btnIndex] = btn
		}
	}

	return mapping, variables
}
