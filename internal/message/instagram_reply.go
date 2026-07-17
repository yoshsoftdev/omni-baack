package message

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	instagramLoginReplyTextNotice = "Pesan terkirim. Quote reply native di Instagram tidak didukung untuk koneksi Instagram Login; konteks reply ditambahkan sebagai teks."
	instagramLoginReplyMediaNotice = "Pesan terkirim. Reply thread di Instagram tidak didukung untuk koneksi Instagram Login."
)

func (s *Service) prepareInstagramReplyDelivery(
	ctx context.Context,
	teamID, conversationID uuid.UUID,
	content string,
	contentType ContentType,
	input SendInput,
) (deliveryContent, notice string) {
	deliveryContent = content
	if !hasReplyIntent(input) {
		return deliveryContent, ""
	}

	if input.StorageObjectID != nil && strings.TrimSpace(input.Content) == "" {
		return deliveryContent, instagramLoginReplyMediaNotice
	}

	notice = instagramLoginReplyTextNotice
	if snippet := s.resolveReplyQuoteSnippet(ctx, teamID, conversationID, input); snippet != "" {
		deliveryContent = formatInstagramQuotedReply(snippet, content)
	}
	return deliveryContent, notice
}

func hasReplyIntent(input SendInput) bool {
	return input.ReplyToMessageID != nil || strings.TrimSpace(input.ReplyToWAMID) != ""
}

func formatInstagramQuotedReply(quoted, reply string) string {
	quoted = strings.TrimSpace(quoted)
	reply = strings.TrimSpace(reply)
	if quoted == "" {
		return reply
	}
	if reply == "" {
		return reply
	}
	quoted = strings.ReplaceAll(quoted, "\n", " ")
	if utf8.RuneCountInString(quoted) > 120 {
		quoted = string([]rune(quoted)[:117]) + "..."
	}
	return fmt.Sprintf("↩ \"%s\"\n%s", quoted, reply)
}

func (s *Service) resolveReplyQuoteSnippet(ctx context.Context, teamID, conversationID uuid.UUID, input SendInput) string {
	if input.ReplyToMessageID != nil {
		target, err := s.messages.FindByID(ctx, teamID, *input.ReplyToMessageID)
		if err == nil && target != nil && target.ConversationID == conversationID {
			return strings.TrimSpace(target.Content)
		}
	}
	replyToMID := strings.TrimSpace(input.ReplyToWAMID)
	if replyToMID == "" {
		return ""
	}
	target, err := s.messages.FindByWAMID(ctx, replyToMID)
	if err == nil && target != nil && target.TeamID == teamID && target.ConversationID == conversationID {
		return strings.TrimSpace(target.Content)
	}
	return ""
}
