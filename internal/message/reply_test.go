package message

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type stubMessageRepo struct {
	byWAMID map[string]*Message
	byKey   map[string]*Message
}

func (s *stubMessageRepo) Create(context.Context, *Message) error { return nil }
func (s *stubMessageRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*Message, error) {
	return nil, repository.ErrNotFound
}
func (s *stubMessageRepo) FindByWAMID(_ context.Context, wamid string) (*Message, error) {
	if m, ok := s.byWAMID[wamid]; ok {
		return m, nil
	}
	return nil, repository.ErrNotFound
}
func (s *stubMessageRepo) FindByWAMIDKey(_ context.Context, teamID, conversationID uuid.UUID, key string) (*Message, error) {
	if m, ok := s.byKey[key]; ok && m.TeamID == teamID && m.ConversationID == conversationID {
		return m, nil
	}
	return nil, repository.ErrNotFound
}
func (s *stubMessageRepo) FindByClientMessageID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*Message, error) {
	return nil, repository.ErrNotFound
}
func (s *stubMessageRepo) ListByConversation(context.Context, uuid.UUID, uuid.UUID, ListFilter, repository.PageParams) (repository.Page[Message], error) {
	return repository.Page[Message]{}, nil
}
func (s *stubMessageRepo) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, Status) error { return nil }
func (s *stubMessageRepo) MarkDeletedByID(context.Context, uuid.UUID, uuid.UUID, DeletedBySource) error {
	return nil
}
func (s *stubMessageRepo) MarkDeletedByWAMID(context.Context, uuid.UUID, string, DeletedBySource) error {
	return nil
}

func TestResolveReplyTarget_WAToWAContext(t *testing.T) {
	teamID := uuid.New()
	conversationID := uuid.New()
	originalID := uuid.New()
	originalWAMID := "wamid.HBgNNjI4MjI4ODgxMjAwMxUCABIYFDNCMkJGRjRFNTA3RDNBOTJBQUZFAA=="
	replyContextWAMID := "wamid.HBgUSUQuMzcwMzA4MjQ4MDk4MzgxNDYVFAASGBQzQjJCRkY0RTUwN0QzQTkyQUFGRQA="

	repo := &stubMessageRepo{
		byWAMID: map[string]*Message{
			originalWAMID: {
				ID:             originalID,
				TeamID:         teamID,
				ConversationID: conversationID,
				WAMID:          &originalWAMID,
			},
		},
		byKey: map[string]*Message{
			"3B2BFF4E507D3A92AAFE": {
				ID:             originalID,
				TeamID:         teamID,
				ConversationID: conversationID,
				WAMID:          &originalWAMID,
			},
		},
	}
	svc := &Service{messages: repo}

	got := svc.resolveReplyTarget(context.Background(), teamID, conversationID, replyContextWAMID)
	if got == nil || *got != originalID {
		t.Fatalf("resolveReplyTarget() = %v, want %v", got, originalID)
	}
}
