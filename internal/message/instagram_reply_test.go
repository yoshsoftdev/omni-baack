package message

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFormatInstagramQuotedReply(t *testing.T) {
	got := formatInstagramQuotedReply("Halo kak", "Siap")
	want := "↩ \"Halo kak\"\nSiap"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatInstagramQuotedReply_TruncatesLongQuote(t *testing.T) {
	long := strings.Repeat("a", 150)
	got := formatInstagramQuotedReply(long, "ok")
	if !strings.HasPrefix(got, "↩ \"") {
		t.Fatalf("missing prefix: %q", got)
	}
	if utf8Count(got) > 140 {
		t.Fatalf("quote too long: %d chars", utf8Count(got))
	}
}

func TestPrepareInstagramReplyDelivery_TextReply(t *testing.T) {
	teamID := uuid.New()
	conversationID := uuid.New()
	targetID := uuid.New()
	repo := &stubMessageRepo{
		byWAMID: map[string]*Message{
			"mid-1": {
				ID:             targetID,
				TeamID:         teamID,
				ConversationID: conversationID,
				Content:        "Pesan asli",
			},
		},
	}
	svc := &Service{messages: repo}
	replyID := targetID

	delivery, notice := svc.prepareInstagramReplyDelivery(
		context.Background(),
		teamID,
		conversationID,
		"Balasan",
		ContentText,
		SendInput{ReplyToMessageID: &replyID},
	)
	if delivery != "↩ \"Pesan asli\"\nBalasan" {
		t.Fatalf("delivery=%q", delivery)
	}
	if notice != instagramLoginReplyTextNotice {
		t.Fatalf("notice=%q", notice)
	}
}

func TestPrepareInstagramReplyDelivery_NoReplyIntent(t *testing.T) {
	svc := &Service{messages: &stubMessageRepo{}}
	delivery, notice := svc.prepareInstagramReplyDelivery(
		context.Background(),
		uuid.New(),
		uuid.New(),
		"Halo",
		ContentText,
		SendInput{},
	)
	if delivery != "Halo" || notice != "" {
		t.Fatalf("delivery=%q notice=%q", delivery, notice)
	}
}

func utf8Count(s string) int {
	return len([]rune(s))
}

func TestResolveReplyQuoteSnippet_FromWAMID(t *testing.T) {
	teamID := uuid.New()
	conversationID := uuid.New()
	mid := "mid-parent"
	svc := &Service{messages: &stubMessageRepo{
		byWAMID: map[string]*Message{
			mid: {
				TeamID:         teamID,
				ConversationID: conversationID,
				Content:        "from mid",
			},
		},
	}}
	got := svc.resolveReplyQuoteSnippet(context.Background(), teamID, conversationID, SendInput{ReplyToWAMID: mid})
	if got != "from mid" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveReplyQuoteSnippet_Missing(t *testing.T) {
	svc := &Service{messages: &stubMessageRepo{}}
	got := svc.resolveReplyQuoteSnippet(context.Background(), uuid.New(), uuid.New(), SendInput{ReplyToWAMID: "missing"})
	if got != "" {
		t.Fatalf("got %q", got)
	}
}
