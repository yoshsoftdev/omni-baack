package eventconsumer

import (
	"encoding/json"
	"testing"
)

func TestExtractInboundMediaRef(t *testing.T) {
	t.Run("document with filename", func(t *testing.T) {
		raw := json.RawMessage(`{"document":{"id":"media-1","filename":"invoice.pdf"}}`)
		mediaID, filename := extractInboundMediaRef("document", raw)
		if mediaID != "media-1" {
			t.Fatalf("mediaID = %q", mediaID)
		}
		if filename != "invoice.pdf" {
			t.Fatalf("filename = %q", filename)
		}
	})

	t.Run("image without filename", func(t *testing.T) {
		raw := json.RawMessage(`{"image":{"id":"media-image"}}`)
		mediaID, filename := extractInboundMediaRef("image", raw)
		if mediaID != "media-image" {
			t.Fatalf("mediaID = %q", mediaID)
		}
		if filename != "" {
			t.Fatalf("filename = %q", filename)
		}
	})
}

func TestNormalizeInboundMIME(t *testing.T) {
	if got := normalizeInboundMIME("", "image"); got != "image/jpeg" {
		t.Fatalf("image fallback = %q", got)
	}
	if got := normalizeInboundMIME("application/octet-stream", "sticker"); got != "image/webp" {
		t.Fatalf("sticker fallback = %q", got)
	}
	if got := normalizeInboundMIME("image/png", "image"); got != "image/png" {
		t.Fatalf("meta mime preserved = %q", got)
	}
}

func TestBuildInboundFilename(t *testing.T) {
	if got := buildInboundFilename(" report.pdf ", "abc", "application/pdf"); got != "report.pdf" {
		t.Fatalf("unexpected fallback filename: %q", got)
	}
	if got := buildInboundFilename("", "media-id", "image/jpeg"); got != "media-id.jpg" {
		t.Fatalf("unexpected jpeg filename: %q", got)
	}
	if got := buildInboundFilename("", "media-id", "application/octet-stream"); got != "media-id" {
		t.Fatalf("unexpected default filename: %q", got)
	}
}
