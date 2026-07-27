package message_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Guard: live Inbox outbound must not call pkg/meta send APIs directly.
func TestOutboundProductionSourcesAvoidPkgMetaSend(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "message")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	forbiddenImports := []string{
		`"github.com/omnichannel/omni-chan-v2/pkg/meta"`,
		`metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"`,
	}
	// inbound may parse webhook payloads; outbound + legacy stub must not import pkg/meta.
	skipFiles := map[string]struct{}{
		"inbound.go": {},
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, skip := skipFiles[name]; skip {
			continue
		}
		path := filepath.Join(root, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.ParseFile(fset, path, src, parser.ImportsOnly); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		body := string(src)
		for _, needle := range forbiddenImports {
			if strings.Contains(body, needle) {
				t.Fatalf("%s must not import pkg/meta (use internal/outbound Channel)", path)
			}
		}
		if strings.Contains(body, "SendTextMessageWithContext") ||
			strings.Contains(body, "SendMediaMessageWithContext") ||
			strings.Contains(body, "SendInstagramTextMessage") ||
			strings.Contains(body, "UploadMedia") {
			t.Fatalf("%s must not call Meta send/upload APIs directly", path)
		}
	}
}
