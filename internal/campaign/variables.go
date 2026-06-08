package campaign

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/omnichannel/omni-chan-v2/internal/contact"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type VariableMapping struct {
	Body map[string]string `json:"body"`
}

func parseVariableMapping(raw json.RawMessage) (VariableMapping, error) {
	if len(raw) == 0 {
		return VariableMapping{}, nil
	}
	var mapping VariableMapping
	if err := json.Unmarshal(raw, &mapping); err != nil {
		return VariableMapping{}, fmt.Errorf("invalid variable_mapping: %w", err)
	}
	return mapping, nil
}

func resolveContactField(c *contact.Contact, field string) string {
	switch field {
	case "name":
		return c.Name
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

func buildTemplateComponents(mapping VariableMapping, c *contact.Contact) []metaclient.TemplateComponent {
	if len(mapping.Body) == 0 {
		return nil
	}

	keys := make([]int, 0, len(mapping.Body))
	for key := range mapping.Body {
		if n, err := strconv.Atoi(key); err == nil {
			keys = append(keys, n)
		}
	}
	sort.Ints(keys)

	params := make([]metaclient.TemplateParameter, 0, len(keys))
	for _, n := range keys {
		field := mapping.Body[strconv.Itoa(n)]
		params = append(params, metaclient.TemplateParameter{
			Type: "text",
			Text: resolveContactField(c, field),
		})
	}

	if len(params) == 0 {
		return nil
	}
	return []metaclient.TemplateComponent{{
		Type:       "body",
		Parameters: params,
	}}
}
