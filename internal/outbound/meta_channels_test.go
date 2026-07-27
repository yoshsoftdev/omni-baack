package outbound_test

import (
	"context"
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/outbound"
)

type stubMeta struct {
	lastWAText  string
	lastWAMedia string
	lastIGText  string
	lastIGMedia string
	deleted     string
}

func (s *stubMeta) SendTextMessageWithContext(_ context.Context, _, _, to, body, _ string) (string, error) {
	s.lastWAText = to + ":" + body
	return "wamid-text", nil
}
func (s *stubMeta) SendMediaMessageWithContext(_ context.Context, _, _, _, _, mediaID, _, _, _ string) (string, error) {
	s.lastWAMedia = mediaID
	return "wamid-media", nil
}
func (s *stubMeta) DeleteMessage(_ context.Context, _, _, wamid string) error {
	s.deleted = wamid
	return nil
}
func (s *stubMeta) SendInstagramTextMessage(_ context.Context, _, _, to, body, _ string) (string, error) {
	s.lastIGText = to + ":" + body
	return "mid-text", nil
}
func (s *stubMeta) SendInstagramMediaMessage(_ context.Context, _, _, to, _, url, _, _ string) (string, error) {
	s.lastIGMedia = to + ":" + url
	return "mid-media", nil
}

func TestWhatsAppChannel_SendTextMediaDelete(t *testing.T) {
	t.Parallel()
	meta := &stubMeta{}
	ch := outbound.NewWhatsAppChannel(meta)
	creds := outbound.Credentials{AccessToken: "tok", PhoneNumberID: "pnid"}

	id, err := ch.SendText(context.Background(), creds, outbound.TextMessage{To: "6281", Body: "hi"})
	if err != nil || id != "wamid-text" {
		t.Fatalf("text: %s %v", id, err)
	}
	id, err = ch.SendMedia(context.Background(), creds, outbound.MediaMessage{
		To: "6281", Type: "image", ProviderMediaID: "99",
	})
	if err != nil || id != "wamid-media" || meta.lastWAMedia != "99" {
		t.Fatalf("media: %s %v meta=%s", id, err, meta.lastWAMedia)
	}
	if err := ch.DeleteMessage(context.Background(), creds, "wamid.x"); err != nil || meta.deleted != "wamid.x" {
		t.Fatalf("delete: %v deleted=%s", err, meta.deleted)
	}
}

func TestInstagramChannel_SendTextMedia(t *testing.T) {
	t.Parallel()
	meta := &stubMeta{}
	ch := outbound.NewInstagramChannel(meta)
	creds := outbound.Credentials{AccessToken: "tok", IGUserID: "ig1"}

	id, err := ch.SendText(context.Background(), creds, outbound.TextMessage{To: "scoped", Body: "hi"})
	if err != nil || id != "mid-text" {
		t.Fatalf("text: %s %v", id, err)
	}
	id, err = ch.SendMedia(context.Background(), creds, outbound.MediaMessage{
		To: "scoped", Type: "image", MediaURL: "https://x/a.jpg", MIMEType: "image/jpeg",
	})
	if err != nil || id != "mid-media" {
		t.Fatalf("media: %s %v", id, err)
	}
	_, err = ch.SendMedia(context.Background(), creds, outbound.MediaMessage{
		To: "scoped", Type: "image", MediaURL: "https://x/a.webp", MIMEType: "image/webp",
	})
	if err == nil {
		t.Fatal("expected webp rejection")
	}
}
