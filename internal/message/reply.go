package message

import (
	"context"
	"strings"

	"github.com/google/uuid"
	sharedwamid "github.com/omnichannel/omni-chan-v2/shared/wamid"
)

func (s *Service) resolveReplyTarget(ctx context.Context, teamID, conversationID uuid.UUID, replyToWAMID string) *uuid.UUID {
	replyToWAMID = strings.TrimSpace(replyToWAMID)
	if replyToWAMID == "" {
		return nil
	}

	if target, err := s.messages.FindByWAMID(ctx, replyToWAMID); err == nil && target != nil && target.TeamID == teamID {
		return &target.ID
	}

	key := sharedwamid.ExtractMessageKey(replyToWAMID)
	if key == "" {
		return nil
	}
	target, err := s.messages.FindByWAMIDKey(ctx, teamID, conversationID, key)
	if err != nil || target == nil {
		return nil
	}
	return &target.ID
}
