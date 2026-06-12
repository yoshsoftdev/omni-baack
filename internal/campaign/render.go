package campaign

import (
	"fmt"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

// renderTemplateContent builds inbox-visible text from template body and variable mapping.
func renderTemplateContent(tpl *template.Template, mapping VariableMapping, c *contact.Contact) string {
	body := strings.TrimSpace(template.ExtractPreview(tpl.Components))
	if body == "" {
		return fmt.Sprintf("[Template: %s]", tpl.Name)
	}

	for key, field := range mapping.Body {
		placeholder := fmt.Sprintf("{{%s}}", key)
		body = strings.ReplaceAll(body, placeholder, resolveContactField(c, field))
	}

	// Replace any unresolved placeholders with empty string.
	for i := 1; i <= 10; i++ {
		placeholder := fmt.Sprintf("{{%d}}", i)
		body = strings.ReplaceAll(body, placeholder, "")
	}

	return strings.TrimSpace(body)
}
