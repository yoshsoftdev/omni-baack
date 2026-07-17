package eventconsumer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/pkg/meta"
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
	if got := normalizeInboundMIME("application/octet-stream", "sticker"); got != "image/jpeg" {
		t.Fatalf("sticker fallback = %q", got)
	}
	if got := normalizeInboundMIME("image/png", "image"); got != "image/png" {
		t.Fatalf("meta mime preserved = %q", got)
	}
	if got := normalizeInboundMIME("", "ig_post"); got != "image/jpeg" {
		t.Fatalf("ig_post fallback = %q", got)
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

func TestExtractInstagramAttachmentURL(t *testing.T) {
	t.Run("ig_post url and title", func(t *testing.T) {
		raw := json.RawMessage(`{"attachments":[{"type":"ig_post","payload":{"url":"https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=1","title":"Hello post"}}]}`)
		mediaURL, attType, filename := extractInstagramAttachmentURL(raw)
		if mediaURL == "" || !strings.Contains(mediaURL, "lookaside.fbsbx.com") {
			t.Fatalf("mediaURL = %q", mediaURL)
		}
		if attType != "ig_post" {
			t.Fatalf("attType = %q", attType)
		}
		if filename == "" {
			t.Fatalf("expected filename from title")
		}
	})

	t.Run("ig_story media url", func(t *testing.T) {
		raw := json.RawMessage(`{"attachments":[{"type":"ig_story","payload":{"story_media_id":"1","story_media_url":"https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=2"}}]}`)
		mediaURL, attType, _ := extractInstagramAttachmentURL(raw)
		if mediaURL == "" {
			t.Fatalf("mediaURL empty")
		}
		if attType != "ig_story" {
			t.Fatalf("attType = %q", attType)
		}
	})
}

func TestPreferAttachmentContentType(t *testing.T) {
	got := preferAttachmentContentType(message.ContentOther, "image/jpeg", "ig_post")
	if got != message.ContentOther {
		t.Fatalf("ig_post should stay other, got=%s", got)
	}
	got = preferAttachmentContentType(message.ContentOther, "video/mp4", "ig_reel")
	if got != message.ContentOther {
		t.Fatalf("ig_reel should stay other, got=%s", got)
	}
	got = preferAttachmentContentType(message.ContentOther, "image/jpeg", "story_mention")
	if got != message.ContentImage {
		t.Fatalf("story_mention got=%s", got)
	}
}

func TestIsAllowedMetaMediaURL(t *testing.T) {
	if !meta.IsAllowedMetaMediaURL("https://lookaside.fbsbx.com/ig_messaging_cdn/?asset_id=1") {
		t.Fatal("expected lookaside allowed")
	}
	if meta.IsAllowedMetaMediaURL("https://evil.example.com/x") {
		t.Fatal("expected external host blocked")
	}
}
