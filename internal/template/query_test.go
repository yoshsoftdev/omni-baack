package template

import "testing"

func TestNormalizeListFilter(t *testing.T) {
	t.Parallel()
	got := NormalizeListFilter(ListFilter{
		Search:   "  welcome ",
		Language: "en",
		Category: "marketing",
	})
	if got.Search != "welcome" {
		t.Fatalf("search = %q", got.Search)
	}
	if got.Language != "en" {
		t.Fatalf("language = %q", got.Language)
	}
	if got.Category != "MARKETING" {
		t.Fatalf("category = %q", got.Category)
	}

	cleared := NormalizeListFilter(ListFilter{Language: "all", Category: "all"})
	if cleared.Language != "" || cleared.Category != "" {
		t.Fatalf("all should clear filters: %+v", cleared)
	}
}
