package inbox

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// WindowRecontactMessageSender delivers an agent free-text reply as the team owner.
type WindowRecontactMessageSender interface {
	SendMessage(
		ctx context.Context,
		teamID, userID uuid.UUID,
		actorRole string,
		conversationID uuid.UUID,
		input message.SendInput,
	) (*message.Message, error)
}

// WindowRecontactConversationStore loads conversations for outbound sync.
type WindowRecontactConversationStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*conversation.Conversation, error)
}

// WindowRecontactInboxSync applies outbound conversation + realtime side effects.
type WindowRecontactInboxSync interface {
	HandleOutbound(ctx context.Context, p OutboundParams) (*conversation.Conversation, error)
}

// WindowRecontactService manages settings and the periodic auto-send worker.
type WindowRecontactService struct {
	store         WindowRecontactStore
	teams         team.Repository
	rbac          *rbac.Service
	messages      WindowRecontactMessageSender
	conversations WindowRecontactConversationStore
	inbox         WindowRecontactInboxSync
	log           *slog.Logger
	interval      time.Duration
	batchSize     int
}

func NewWindowRecontactService(
	store WindowRecontactStore,
	teams team.Repository,
	rbacSvc *rbac.Service,
	messages WindowRecontactMessageSender,
	conversations WindowRecontactConversationStore,
	inboxSync WindowRecontactInboxSync,
	log *slog.Logger,
) *WindowRecontactService {
	if log == nil {
		log = slog.Default()
	}
	return &WindowRecontactService{
		store:         store,
		teams:         teams,
		rbac:          rbacSvc,
		messages:      messages,
		conversations: conversations,
		inbox:         inboxSync,
		log:           log,
		interval:      1 * time.Minute,
		batchSize:     25,
	}
}

func (s *WindowRecontactService) GetSettings(
	ctx context.Context,
	teamID, userID uuid.UUID,
	role string,
) (WindowRecontactSettings, error) {
	if err := s.authorizeView(ctx, teamID, userID, role); err != nil {
		return WindowRecontactSettings{}, err
	}
	return s.store.GetWindowRecontactSettings(ctx, teamID)
}

func (s *WindowRecontactService) UpdateSettings(
	ctx context.Context,
	teamID, userID uuid.UUID,
	role string,
	settings WindowRecontactSettings,
) (WindowRecontactSettings, error) {
	if err := s.authorizeManage(ctx, teamID, userID, role); err != nil {
		return WindowRecontactSettings{}, err
	}
	return s.store.UpsertWindowRecontactSettings(ctx, teamID, settings)
}

func (s *WindowRecontactService) authorizeView(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	if s.rbac != nil {
		ok, err := s.rbac.HasPermission(ctx, role, rbac.PermConversationView)
		if err != nil {
			return err
		}
		if !ok {
			return serviceerrors.ErrForbidden
		}
	}
	return nil
}

func (s *WindowRecontactService) authorizeManage(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	normalized := strings.ToLower(strings.TrimSpace(role))
	if normalized != "owner" && normalized != "admin" {
		return serviceerrors.ErrForbidden
	}
	return nil
}

func (s *WindowRecontactService) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	if s.teams == nil {
		return nil
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		return serviceerrors.MapRepository(err)
	}
	return nil
}

// Start runs the due-recontact poller until ctx is cancelled.
func (s *WindowRecontactService) Start(ctx context.Context) {
	if s == nil || s.store == nil || s.messages == nil {
		return
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *WindowRecontactService) runOnce(ctx context.Context) {
	candidates, err := s.store.ListDueWindowRecontacts(ctx, s.batchSize, time.Now().UTC())
	if err != nil {
		s.log.Error("window recontact list failed", "error", err)
		return
	}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return
		}
		if err := s.processCandidate(ctx, candidate); err != nil {
			s.log.Warn("window recontact send failed",
				"team_id", candidate.TeamID,
				"conversation_id", candidate.ConversationID,
				"error", err,
			)
		}
	}
}

func (s *WindowRecontactService) processCandidate(ctx context.Context, candidate WindowRecontactCandidate) error {
	text := strings.TrimSpace(candidate.Text)
	if text == "" {
		return nil
	}

	claimed, sendID, err := s.store.TryClaimWindowRecontact(
		ctx,
		candidate.TeamID,
		candidate.ConversationID,
		candidate.LastCustomerMessageAt,
	)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	ownerID, err := s.store.FindTeamOwnerUserID(ctx, candidate.TeamID)
	if err != nil {
		_ = s.store.DeleteWindowRecontactClaim(ctx, sendID)
		return err
	}

	msg, err := s.messages.SendMessage(
		ctx,
		candidate.TeamID,
		ownerID,
		string(team.RoleOwner),
		candidate.ConversationID,
		message.SendInput{Content: text},
	)
	if err != nil {
		_ = s.store.DeleteWindowRecontactClaim(ctx, sendID)
		return err
	}

	if s.inbox != nil && s.conversations != nil {
		conv, findErr := s.conversations.FindByID(ctx, candidate.TeamID, candidate.ConversationID)
		if findErr == nil && conv != nil {
			_, _ = s.inbox.HandleOutbound(ctx, OutboundParams{
				TeamID:       candidate.TeamID,
				AgentID:      ownerID,
				Conversation: conv,
				Message:      msg,
			})
		}
	}

	if markErr := s.store.MarkWindowRecontactSent(ctx, sendID, msg.ID); markErr != nil {
		s.log.Warn("window recontact mark sent failed", "send_id", sendID, "error", markErr)
	}
	return nil
}
