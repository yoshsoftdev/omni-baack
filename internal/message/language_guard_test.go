package message_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Guard: Inbox-adjacent packages must not reintroduce Meta send-language hardcodes.
func TestNoHardcodedOTPTemplateLanguageInInboxPath(t *testing.T) {
	t.Parallel()
	roots := []string{
		filepath.Join("..", "message"),
		filepath.Join("..", "inbox"),
		filepath.Join("..", "otp"),
	}
	forbidden := []string{
		"OTPTemplateLanguage",
		`language: "en_EN"`,
		`"en_EN"`,
	}
	fset := token.NewFileSet()
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("readdir %s: %v", root, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(root, name)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Parse to ensure we only scan real Go; then string-scan production sources.
			if _, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			body := string(src)
			for _, needle := range forbidden {
				if strings.Contains(body, needle) {
					// Allow comments that document the ban.
					if needle == `"en_EN"` && strings.Contains(body, "never hardcode") {
						continue
					}
					t.Fatalf("%s must not contain %q (use LanguageResolver / MetaSendLanguage)", path, needle)
				}
			}
		}
	}
}
