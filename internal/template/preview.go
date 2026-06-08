package template

import (
	"encoding/json"
	"strings"
)

type metaComponent struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Format  string `json:"format"`
	Buttons []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"buttons"`
}

func ExtractPreview(components json.RawMessage) string {
	if len(components) == 0 {
		return ""
	}
	var items []metaComponent
	if err := json.Unmarshal(components, &items); err != nil {
		return ""
	}
	for _, item := range items {
		if strings.EqualFold(item.Type, "BODY") && strings.TrimSpace(item.Text) != "" {
			text := strings.TrimSpace(item.Text)
			if len(text) > 120 {
				return text[:117] + "..."
			}
			return text
		}
	}
	return ""
}
