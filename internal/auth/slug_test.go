package auth

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Acme Corp", "acme-corp"},
		{"  Hello   World ", "hello-world"},
		{"", "workspace"},
	}

	for _, tc := range tests {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
