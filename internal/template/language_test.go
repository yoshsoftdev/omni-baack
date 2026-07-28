package template

import "testing"

func TestLanguageResolverFromMeta(t *testing.T) {
	t.Parallel()
	r := DefaultLanguageResolver()
	cases := map[string]string{
		"":      "en_US",
		"en":    "en",
		"EN":    "en",
		"en_US": "en_US",
		"en_EN": "en_US",
		"id":    "id",
		"id_ID": "id",
		"pt_BR": "pt_BR",
		"pt":    "pt",
	}
	for in, want := range cases {
		if got := r.ForStorageFromMeta(in); got != want {
			t.Fatalf("ForStorageFromMeta(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLanguageResolverFromCreate(t *testing.T) {
	t.Parallel()
	r := DefaultLanguageResolver()
	if got := r.ForStorageFromCreate(""); got != "id" {
		t.Fatalf("empty create default = %s", got)
	}
	if got := r.ForStorageFromCreate("en"); got != "en" {
		t.Fatalf("en = %s", got)
	}
	if got := r.ForStorageFromCreate("en_US"); got != "en_US" {
		t.Fatalf("en_US = %s", got)
	}
}

func TestLanguageResolverForMetaSend(t *testing.T) {
	t.Parallel()
	r := DefaultLanguageResolver()
	cases := map[string]string{
		"":      "en_US",
		"en_EN": "en_US",
		"en":    "en",
		"en_US": "en_US",
		"id_ID": "id",
		"id":    "id",
		"pt_BR": "pt_BR",
	}
	for in, want := range cases {
		if got := r.ForMetaSend(in); got != want {
			t.Fatalf("ForMetaSend(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLanguageResolverMatchesFilter(t *testing.T) {
	t.Parallel()
	r := DefaultLanguageResolver()
	if !r.MatchesFilter("en_US", "en") {
		t.Fatal("en_US should match filter en")
	}
	if !r.MatchesFilter("en", "en_US") {
		t.Fatal("en should match filter en_US")
	}
	if !r.MatchesFilter("id", "id_ID") {
		t.Fatal("id should match filter id_ID")
	}
	if r.MatchesFilter("en_US", "id") {
		t.Fatal("en_US must not match id")
	}
}
