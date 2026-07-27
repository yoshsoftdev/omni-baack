package template

import (
	"bytes"
	"encoding/json"
)

// templateDriftChanged reports whether Meta-visible template data differs after merge.
// Callers must run MergeSyncedTemplate before comparing so internal keys do not cause false drift.
func templateDriftChanged(existing, incoming *Template) bool {
	if existing == nil {
		return false
	}
	if existing.Name != incoming.Name ||
		existing.Language != incoming.Language ||
		existing.Status != incoming.Status {
		return true
	}
	if !jsonEqual(existing.Components, incoming.Components) {
		return true
	}
	if stringPtr(existing.Category) != stringPtr(incoming.Category) {
		return true
	}
	if stringPtr(existing.Preview) != stringPtr(incoming.Preview) {
		return true
	}
	return existing.VariableCount != incoming.VariableCount
}

func jsonEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
}

func stringPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
