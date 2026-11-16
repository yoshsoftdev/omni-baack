package inbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Ledger kinds: at most one of each per inbound message.
const (
	AutoReplyWelcome = "welcome"
	AutoReplyKeyword = "keyword"
)

// AutoReplyInbound is a newly stored inbound message (never a redelivery).
type AutoReplyInbound struct {
	TeamID         uuid.UUID
	ConversationID uuid.UUID
	// ConversationCreated: this message started the customer's first conversation on the channel.
	ConversationCreated bool
	// LastHumanReplyAt is the conversation's last agent message; keyword replies pause for
	// HumanHandoverWindow after it, so the bot never talks over an agent.
	LastHumanReplyAt *time.Time
	Message          *message.Message
}

// HumanHandoverWindow: after a human agent replies, keyword replies stay quiet this long.
const HumanHandoverWindow = 24 * time.Hour

func humanActive(lastHumanReplyAt *time.Time, at time.Time) bool {
	return lastHumanReplyAt != nil && at.Sub(*lastHumanReplyAt) < HumanHandoverWindow
}

// AutoReplyService manages the welcome message and keyword rules, and sends them on inbound messages.
type AutoReplyService struct {
	store         AutoReplyStore
	teams         team.Repository
	rbac          *rbac.Service
	messages      WindowRecontactMessageSender
	conversations WindowRecontactConversationStore
	inbox         WindowRecontactInboxSync
	log           *slog.Logger
}

func NewAutoReplyService(
	store AutoReplyStore,
	teams team.Repository,
	rbacSvc *rbac.Service,
	messages WindowRecontactMessageSender,
	conversations WindowRecontactConversationStore,
	inboxSync WindowRecontactInboxSync,
	log *slog.Logger,
) *AutoReplyService {
	if log == nil {
		log = slog.Default()
	}
	return &AutoReplyService{store: store, teams: teams, rbac: rbacSvc, messages: messages, conversations: conversations, inbox: inboxSync, log: log}
}

func (s *AutoReplyService) authorize(ctx context.Context, teamID, userID uuid.UUID, role, perm string) error {
	if s.rbac == nil || !s.rbac.Require(role, perm) {
		return serviceerrors.ErrForbidden
	}
	if s.teams != nil {
		if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
			return serviceerrors.MapRepository(err)
		}
	}
	return nil
}

func (s *AutoReplyService) GetWelcome(ctx context.Context, teamID, userID uuid.UUID, role string) (WelcomeSettings, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermAutomationView); err != nil {
		return WelcomeSettings{}, err
	}
	return s.store.GetWelcomeSettings(ctx, teamID)
}

func (s *AutoReplyService) UpdateWelcome(ctx context.Context, teamID, userID uuid.UUID, role string, in WelcomeSettings) (WelcomeSettings, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermAutomationUpdate); err != nil {
		return WelcomeSettings{}, err
	}
	settings, err := NormalizeWelcomeSettings(in)
	if err != nil {
		return WelcomeSettings{}, err
	}
	if err := s.store.UpsertWelcomeSettings(ctx, teamID, settings); err != nil {
		return WelcomeSettings{}, err
	}
	return settings, nil
}

func (s *AutoReplyService) ListRules(ctx context.Context, teamID, userID uuid.UUID, role string) ([]KeywordRule, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermAutomationView); err != nil {
		return nil, err
	}
	return s.store.ListKeywordRules(ctx, teamID)
}

func (s *AutoReplyService) CreateRule(ctx context.Context, teamID, userID uuid.UUID, role string, in KeywordRule) (*KeywordRule, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermAutomationCreate); err != nil {
		return nil, err
	}
	rule, err := NormalizeKeywordRule(in)
	if err != nil {
		return nil, err
	}
	n, err := s.store.CountKeywordRules(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if n >= MaxKeywordRules {
		return nil, serviceerrors.NewValidation("rules", "at most 100 keyword rules per team")
	}
	rule.ID = uuid.New()
	if err := s.store.CreateKeywordRule(ctx, teamID, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *AutoReplyService) UpdateRule(ctx context.Context, teamID, userID uuid.UUID, role string, id uuid.UUID, in KeywordRule) (*KeywordRule, error) {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermAutomationUpdate); err != nil {
		return nil, err
	}
	rule, err := NormalizeKeywordRule(in)
	if err != nil {
		return nil, err
	}
	rule.ID = id
	if err := s.store.UpdateKeywordRule(ctx, teamID, &rule); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return &rule, nil
}

func (s *AutoReplyService) DeleteRule(ctx context.Context, teamID, userID uuid.UUID, role string, id uuid.UUID) error {
	if err := s.authorize(ctx, teamID, userID, role, rbac.PermAutomationDelete); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.store.DeleteKeywordRule(ctx, teamID, id))
}

// HandleInbound sends the welcome message (first conversation) and a keyword reply (text message,
// no human agent assigned). Failures are logged, never returned: inbound ingestion must not fail
// because an automatic reply could not be sent.
func (s *AutoReplyService) HandleInbound(ctx context.Context, in AutoReplyInbound) {
	if s == nil || s.store == nil || s.messages == nil || in.Message == nil {
		return
	}
	if in.ConversationCreated {
		welcome, err := s.store.GetWelcomeSettings(ctx, in.TeamID)
		if err != nil {
			s.log.Warn("welcome settings load failed", "team_id", in.TeamID, "error", err)
		} else if welcome.Enabled && welcome.Text != "" {
			s.send(ctx, in, AutoReplyWelcome, nil, welcome.Text)
		}
	}
	at := in.Message.ReceivedAt
	if at.IsZero() {
		at = time.Now()
	}
	if in.Message.ContentType != message.ContentText || humanActive(in.LastHumanReplyAt, at) {
		return
	}
	rules, err := s.store.ListKeywordRules(ctx, in.TeamID)
	if err != nil {
		s.log.Warn("keyword rules load failed", "team_id", in.TeamID, "error", err)
		return
	}
	if rule := MatchKeywordRule(rules, in.Message.Content); rule != nil {
		id := rule.ID
		s.send(ctx, in, AutoReplyKeyword, &id, rule.ReplyText)
	}
}

func (s *AutoReplyService) send(ctx context.Context, in AutoReplyInbound, kind string, ruleID *uuid.UUID, text string) {
	log := s.log.With("team_id", in.TeamID, "conversation_id", in.ConversationID, "kind", kind)
	claimed, claimID, err := s.store.TryClaimAutoReply(ctx, in.TeamID, in.ConversationID, in.Message.ID, kind, ruleID)
	if err != nil || !claimed {
		if err != nil {
			log.Warn("auto reply claim failed", "error", err)
		}
		return
	}
	ownerID, err := s.store.FindTeamOwnerUserID(ctx, in.TeamID)
	if err != nil {
		_ = s.store.DeleteAutoReplyClaim(ctx, claimID)
		log.Warn("auto reply owner lookup failed", "error", err)
		return
	}
	msg, err := s.messages.SendMessage(ctx, in.TeamID, ownerID, string(team.RoleOwner), in.ConversationID, message.SendInput{Content: text})
	if err != nil {
		_ = s.store.DeleteAutoReplyClaim(ctx, claimID)
		log.Warn("auto reply send failed", "error", err)
		return
	}
	if s.inbox != nil && s.conversations != nil {
		if conv, findErr := s.conversations.FindByID(ctx, in.TeamID, in.ConversationID); findErr == nil && conv != nil {
			_, _ = s.inbox.HandleOutbound(ctx, OutboundParams{TeamID: in.TeamID, AgentID: ownerID, Conversation: conv, Message: msg, Automated: true})
		}
	}
	if err := s.store.MarkAutoReplySent(ctx, claimID, msg.ID); err != nil {
		log.Warn("auto reply mark sent failed", "error", err)
	}
}
