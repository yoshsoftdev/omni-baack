package inbox

import (
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Keyword rule match modes.
const (
	// MatchExact: the whole message equals a keyword (case and spacing ignored).
	MatchExact = "exact"
	// MatchContains: a keyword appears as whole word(s) inside the message.
	MatchContains = "contains"
)

// Limits keep rules cheap to evaluate on every inbound message.
const (
	MaxKeywordRules       = 100
	MaxKeywordsPerRule    = 20
	MaxKeywordLength      = 100
	MaxAutoReplyTextChars = 1000
)

// WelcomeSettings: the message sent when a customer's first conversation on a channel starts.
type WelcomeSettings struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
}

// KeywordRule answers an inbound text message that matches one of its keywords.
type KeywordRule struct {
	ID        uuid.UUID `json:"id"`
	Keywords  []string  `json:"keywords"`
	MatchType string    `json:"match_type"`
	ReplyText string    `json:"reply_text"`
	Enabled   bool      `json:"enabled"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NormalizeWelcomeSettings trims the text and rejects an enabled welcome without one.
func NormalizeWelcomeSettings(in WelcomeSettings) (WelcomeSettings, error) {
	in.Text = strings.TrimSpace(in.Text)
	if in.Enabled && in.Text == "" {
		return WelcomeSettings{}, serviceerrors.NewValidation("text", "is required when the welcome message is enabled")
	}
	if len([]rune(in.Text)) > MaxAutoReplyTextChars {
		return WelcomeSettings{}, serviceerrors.NewValidation("text", "must be at most 1000 characters")
	}
	return in, nil
}

// NormalizeKeywordRule cleans keywords (trimmed, deduplicated case-insensitively) and validates the rule.
func NormalizeKeywordRule(in KeywordRule) (KeywordRule, error) {
	if in.MatchType == "" {
		in.MatchType = MatchContains
	}
	if in.MatchType != MatchExact && in.MatchType != MatchContains {
		return KeywordRule{}, serviceerrors.NewValidation("match_type", "must be exact or contains")
	}
	seen := map[string]bool{}
	keywords := make([]string, 0, len(in.Keywords))
	for _, raw := range in.Keywords {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		if len([]rune(kw)) > MaxKeywordLength {
			return KeywordRule{}, serviceerrors.NewValidation("keywords", "each keyword must be at most 100 characters")
		}
		if key := normalizeForMatch(kw); !seen[key] {
			seen[key] = true
			keywords = append(keywords, kw)
		}
	}
	if len(keywords) == 0 {
		return KeywordRule{}, serviceerrors.NewValidation("keywords", "at least one keyword is required")
	}
	if len(keywords) > MaxKeywordsPerRule {
		return KeywordRule{}, serviceerrors.NewValidation("keywords", "at most 20 keywords per rule")
	}
	in.Keywords = keywords
	in.ReplyText = strings.TrimSpace(in.ReplyText)
	if in.ReplyText == "" {
		return KeywordRule{}, serviceerrors.NewValidation("reply_text", "is required")
	}
	if len([]rune(in.ReplyText)) > MaxAutoReplyTextChars {
		return KeywordRule{}, serviceerrors.NewValidation("reply_text", "must be at most 1000 characters")
	}
	return in, nil
}

// normalizeForMatch lowercases and turns punctuation into single spaces ("Harga?!" → "harga").
func normalizeForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// MatchKeywordRule returns the first enabled rule (in the given order) matching text, or nil.
func MatchKeywordRule(rules []KeywordRule, text string) *KeywordRule {
	msg := normalizeForMatch(text)
	if msg == "" {
		return nil
	}
	padded := " " + msg + " "
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled {
			continue
		}
		for _, raw := range rule.Keywords {
			kw := normalizeForMatch(raw)
			if kw == "" {
				continue
			}
			if (rule.MatchType == MatchExact && msg == kw) ||
				(rule.MatchType == MatchContains && strings.Contains(padded, " "+kw+" ")) {
				return rule
			}
		}
	}
	return nil
}
