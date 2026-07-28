package template

import (
	"strings"
)

// LanguageResolver normalizes and maps WhatsApp template locales.
// All create/sync/send/filter language handling MUST go through this type.
type LanguageResolver struct{}

// DefaultLanguageResolver returns the shared resolver instance.
func DefaultLanguageResolver() LanguageResolver {
	return LanguageResolver{}
}

// ForStorageFromMeta maps a Meta list/sync language into the stored domain form.
// Preserve Meta's real locale: en and en_US are distinct send codes (#132001 if swapped).
// Only rewrite known legacy artifacts (en_EN).
func (LanguageResolver) ForStorageFromMeta(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "en_US"
	}
	switch canonicalizeLocaleKey(raw) {
	case "en_en":
		return "en_US"
	case "en":
		return "en"
	case "en_us":
		return "en_US"
	case "id", "id_id":
		return "id"
	default:
		return normalizeLocaleCase(raw)
	}
}

// ForStorageFromCreate maps UI/API create input into the stored domain form.
func (LanguageResolver) ForStorageFromCreate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "id"
	}
	switch canonicalizeLocaleKey(raw) {
	case "id", "id_id":
		return "id"
	case "en_en":
		return "en_US"
	case "en":
		return "en"
	case "en_us":
		return "en_US"
	default:
		return normalizeLocaleCase(raw)
	}
}

// ForMetaSend returns the language.code value for Meta Cloud send APIs.
// Must match the template's Meta language exactly — do not coerce en ↔ en_US.
func (LanguageResolver) ForMetaSend(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "en_US"
	}
	switch canonicalizeLocaleKey(stored) {
	case "en_en":
		return "en_US"
	case "en":
		return "en"
	case "en_us":
		return "en_US"
	case "id", "id_id":
		return "id"
	default:
		return normalizeLocaleCase(stored)
	}
}

// NormalizeForFilter returns a stable key for list/search language filters.
func (r LanguageResolver) NormalizeForFilter(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return r.ForStorageFromMeta(raw)
}

// MatchesFilter reports whether a stored language matches a filter value (aliases included).
func (r LanguageResolver) MatchesFilter(stored, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" || strings.EqualFold(filter, "all") {
		return true
	}
	a := r.NormalizeForFilter(stored)
	b := r.NormalizeForFilter(filter)
	if a == b {
		return true
	}
	// UI "English" filter matches both Meta codes en and en_US.
	return isEnglishLocale(a) && isEnglishLocale(b)
}

// NormalizedStorageKey is the value stored in templates.normalized_language.
func (r LanguageResolver) NormalizedStorageKey(stored string) string {
	return r.ForStorageFromMeta(stored)
}

func isEnglishLocale(normalized string) bool {
	switch canonicalizeLocaleKey(normalized) {
	case "en", "en_us":
		return true
	default:
		return false
	}
}

func canonicalizeLocaleKey(raw string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "-", "_"))
}

func normalizeLocaleCase(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "-", "_"))
	parts := strings.Split(raw, "_")
	if len(parts) == 1 {
		return strings.ToLower(parts[0])
	}
	if len(parts) >= 2 {
		return strings.ToLower(parts[0]) + "_" + strings.ToUpper(parts[1])
	}
	return raw
}
