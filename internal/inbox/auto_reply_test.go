package inbox

import (
	"strings"
	"testing"
)

func TestMatchKeywordRule(t *testing.T) {
	rules := []KeywordRule{
		{Keywords: []string{"Harga"}, MatchType: MatchContains, Enabled: false, ReplyText: "disabled"},
		{Keywords: []string{"jam buka", "buka jam berapa"}, MatchType: MatchContains, Enabled: true, ReplyText: "hours"},
		{Keywords: []string{"harga", "price list"}, MatchType: MatchContains, Enabled: true, ReplyText: "price"},
		{Keywords: []string{"halo"}, MatchType: MatchExact, Enabled: true, ReplyText: "hi"},
	}
	cases := map[string]string{
		"Berapa HARGA-nya kak?":     "price", // punctuation and case ignored; disabled rule skipped
		"Kak, jam buka toko?":       "hours", // multi-word keyword, first matching rule wins
		"Halo!":                     "hi",    // exact ignores punctuation
		"halo, mau tanya harga":     "price", // exact "halo" does not match a longer message
		"hargadiskon":               "",      // contains matches whole words only
		"   ":                       "",
		"Do you have a PRICE LIST?": "price",
	}
	for text, want := range cases {
		got := ""
		if rule := MatchKeywordRule(rules, text); rule != nil {
			got = rule.ReplyText
		}
		if got != want {
			t.Errorf("MatchKeywordRule(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestNormalizeKeywordRule(t *testing.T) {
	rule, err := NormalizeKeywordRule(KeywordRule{Keywords: []string{" Harga ", "harga", "", "PRICE"}, ReplyText: " ok "})
	if err != nil {
		t.Fatal(err)
	}
	if rule.MatchType != MatchContains || len(rule.Keywords) != 2 || rule.ReplyText != "ok" {
		t.Fatalf("rule = %+v", rule)
	}
	bad := []KeywordRule{
		{Keywords: []string{" "}, ReplyText: "x"},
		{Keywords: []string{"a"}, ReplyText: " "},
		{Keywords: []string{"a"}, ReplyText: "x", MatchType: "regex"},
		{Keywords: []string{strings.Repeat("a", 101)}, ReplyText: "x"},
	}
	for _, in := range bad {
		if _, err := NormalizeKeywordRule(in); err == nil {
			t.Errorf("NormalizeKeywordRule(%+v) accepted", in)
		}
	}
	if _, err := NormalizeWelcomeSettings(WelcomeSettings{Enabled: true, Text: "  "}); err == nil {
		t.Error("enabled welcome without text accepted")
	}
}
