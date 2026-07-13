package channelsync

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/debug"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type MetaClient interface {
	RequestSMBSync(ctx context.Context, phoneNumberID, accessToken, syncType string) (*metapkg.SMBSyncResponse, error)
	GetCoexistenceStatus(ctx context.Context, phoneNumberID, accessToken string) (*metapkg.CoexistenceStatus, error)
	ListMessageTemplatesPage(ctx context.Context, wabaID, accessToken, after string) ([]metapkg.MessageTemplate, string, error)
	SubscribeAppToWABA(ctx context.Context, wabaID, accessToken string) error
}

type TemplateSyncer interface {
	SyncChannel(ctx context.Context, teamID, channelID uuid.UUID) (*template.SyncResult, error)
}

type WorkspaceRefresher interface {
	RefreshReadinessInternal(ctx context.Context, teamID uuid.UUID) error
}

type InboxSync interface {
	HandleOutbound(ctx context.Context, p inbox.OutboundParams) (*conversation.Conversation, error)
}

type Service struct {
	state         Repository
	channels      channel.Repository
	contacts      contact.Repository
	conversations conversation.Repository
	messages      message.Repository
	inboxSync     InboxSync
	teams         team.Repository
	templates     TemplateSyncer
	meta          MetaClient
	cipher        *cryptoutil.TokenCipher
	rbac          *rbac.Service
	readiness     WorkspaceRefresher
}

func NewService(
	state Repository,
	channels channel.Repository,
	contacts contact.Repository,
	conversations conversation.Repository,
	messages message.Repository,
	inboxSync InboxSync,
	teams team.Repository,
	templates TemplateSyncer,
	meta MetaClient,
	cipher *cryptoutil.TokenCipher,
	rbacSvc *rbac.Service,
	readiness WorkspaceRefresher,
) *Service {
	return &Service{
		state:         state,
		channels:      channels,
		contacts:      contacts,
		conversations: conversations,
		messages:      messages,
		inboxSync:     inboxSync,
		teams:         teams,
		templates:     templates,
		meta:          meta,
		cipher:        cipher,
		rbac:          rbacSvc,
		readiness:     readiness,
	}
}

func (s *Service) TeamDataSynced(ctx context.Context, teamID uuid.UUID) (bool, error) {
	return s.state.TeamDataSynced(ctx, teamID)
}

func (s *Service) TeamSyncSummary(ctx context.Context, teamID uuid.UUID) (*TeamSyncSummary, error) {
	return s.state.TeamSyncSummary(ctx, teamID)
}

func (s *Service) GetPermissions(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*Permissions, error) {
	if err := s.authorize(actorRole); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	ch, token, expired, err := s.loadChannelToken(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	state, err := s.state.EnsureState(ctx, ch.ID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	perms, err := s.refreshPermissions(ctx, ch, token, expired, state)
	if err != nil {
		return nil, err
	}
	return &perms, nil
}

func (s *Service) GetStatus(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*StatusResponse, error) {
	if err := s.authorize(actorRole); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	ch, token, expired, err := s.loadChannelToken(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	state, err := s.state.EnsureState(ctx, ch.ID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	perms, err := s.refreshPermissions(ctx, ch, token, expired, state)
	if err != nil {
		return nil, err
	}

	created := state.ContactsCreated
	updated := state.ContactsUpdated
	synced := state.TemplatesSynced
	imported := state.ConversationsImported

	return &StatusResponse{
		Permissions: aggregatePermissionState(perms, expired),
		Contacts: ResourceStatus{
			Status:  state.ContactsStatus,
			Created: &created,
			Updated: &updated,
		},
		Templates: ResourceStatus{
			Status: state.TemplatesStatus,
			Synced: &synced,
		},
		Conversations: ResourceStatus{
			Status:   state.ConversationsStatus,
			Imported: &imported,
		},
		LastSyncAt: state.LastSyncAt,
	}, nil
}

func (s *Service) SyncContacts(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*ContactSyncResult, error) {
	if err := s.authorize(actorRole); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	ch, token, expired, err := s.loadChannelToken(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	state, err := s.state.EnsureState(ctx, ch.ID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	perms, err := s.refreshPermissions(ctx, ch, token, expired, state)
	if err != nil {
		return nil, err
	}
	if expired {
		return nil, serviceerrors.NewPermissionRequired("contacts", "Meta access token has expired. Re-authorize with Meta to continue.")
	}
	if !perms.Contacts {
		return nil, serviceerrors.NewPermissionRequired("contacts", "Additional Meta permission is required to synchronize contacts.")
	}

	state.ContactsStatus = SyncInProgress
	if err := s.state.UpdateState(ctx, state); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}

	if ch.PhoneNumberID != nil && s.meta != nil {
		resp, syncErr := s.meta.RequestSMBSync(ctx, *ch.PhoneNumberID, token, "smb_app_state_sync")
		if syncErr != nil {
			state.ContactsStatus = SyncFailed
			_ = s.state.UpdateState(ctx, state)
			return nil, serviceerrors.NewValidation("meta", "failed to request contact sync from Meta")
		}
		if resp != nil && resp.RequestID != "" {
			reqID := resp.RequestID
			state.ContactsSyncRequestID = &reqID
		}
	}

	result, err := s.processPendingContactEvents(ctx, ch.ID, teamID)
	if err != nil {
		state.ContactsStatus = SyncFailed
		_ = s.state.UpdateState(ctx, state)
		return nil, err
	}

	now := time.Now().UTC()
	state.ContactsStatus = SyncCompleted
	state.ContactsCreated += result.Created
	state.ContactsUpdated += result.Updated
	state.ContactsLastSyncAt = &now
	state.LastSyncAt = &now
	if err := s.state.UpdateState(ctx, state); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	s.refreshReadiness(ctx, teamID)
	return result, nil
}

func (s *Service) SyncConversations(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*ConversationSyncResult, error) {
	if err := s.authorize(actorRole); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	ch, token, expired, err := s.loadChannelToken(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	state, err := s.state.EnsureState(ctx, ch.ID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	perms, err := s.refreshPermissions(ctx, ch, token, expired, state)
	if err != nil {
		return nil, err
	}
	if expired {
		return nil, serviceerrors.NewPermissionRequired("conversation_sync", "Meta access token has expired. Re-authorize with Meta to continue.")
	}
	if !perms.ConversationSync {
		return nil, serviceerrors.NewPermissionRequired("conversation_sync", "Additional Meta permission is required to synchronize conversations.")
	}

	state.ConversationsStatus = SyncInProgress
	_ = s.state.UpdateState(ctx, state)

	imported := 0
	if ch.PhoneNumberID != nil && s.meta != nil {
		resp, syncErr := s.meta.RequestSMBSync(ctx, *ch.PhoneNumberID, token, "history")
		if syncErr == nil && resp != nil && resp.RequestID != "" {
			reqID := resp.RequestID
			state.HistorySyncRequestID = &reqID
		}
	}

	now := time.Now().UTC()
	reason := "No conversations available from Meta."
	out := &ConversationSyncResult{Imported: imported, Reason: &reason}

	// #region agent log
	debug.AgentLog("H5", "channelsync/service.go:SyncConversations", "manual sync completed without waiting webhooks", map[string]any{
		"channel_id": channelID.String(), "imported": imported, "history_request_id": state.HistorySyncRequestID,
	})
	// #endregion

	state.ConversationsStatus = SyncCompleted
	state.ConversationsImported = imported
	state.ConversationsReason = &reason
	state.ConversationsLastSyncAt = &now
	state.LastSyncAt = &now
	if err := s.state.UpdateState(ctx, state); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	s.refreshReadiness(ctx, teamID)
	return out, nil
}

func (s *Service) SyncTemplates(ctx context.Context, teamID, userID uuid.UUID, actorRole string, channelID uuid.UUID) (*template.SyncResult, error) {
	if err := s.authorize(actorRole); err != nil {
		return nil, err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if _, _, _, err := s.loadChannelToken(ctx, teamID, channelID); err != nil {
		return nil, err
	}
	if s.templates == nil {
		return nil, serviceerrors.NewValidation("sync", "template sync is not configured")
	}

	state, err := s.state.EnsureState(ctx, channelID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	state.TemplatesStatus = SyncInProgress
	_ = s.state.UpdateState(ctx, state)

	result, syncErr := s.templates.SyncChannel(ctx, teamID, channelID)
	now := time.Now().UTC()
	if syncErr != nil {
		state.TemplatesStatus = SyncFailed
		_ = s.state.UpdateState(ctx, state)
		return nil, syncErr
	}

	state.TemplatesStatus = SyncCompleted
	state.TemplatesSynced = result.Synced
	state.TemplatesLastSyncAt = &now
	state.LastSyncAt = &now
	perms := state.Permissions
	perms.Templates = true
	state.Permissions = perms
	if err := s.state.UpdateState(ctx, state); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	s.refreshReadiness(ctx, teamID)
	return result, nil
}

func (s *Service) RefreshPermissionsAfterConnect(ctx context.Context, teamID, channelID uuid.UUID, accessToken string) error {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	if ch.IsCoex && ch.WABAID != nil && strings.TrimSpace(accessToken) != "" && s.meta != nil {
		_ = s.meta.SubscribeAppToWABA(ctx, *ch.WABAID, accessToken)
	}
	state, err := s.state.EnsureState(ctx, channelID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}
	_, err = s.refreshPermissions(ctx, ch, accessToken, false, state)
	return err
}

func (s *Service) HandleCoexWebhook(ctx context.Context, events *metapkg.CoexWebhookEvents) error {
	// #region agent log
	debug.AgentLog("H3", "channelsync/service.go:HandleCoexWebhook", "coex webhook handling", map[string]any{
		"history_events": len(events.HistoryEvents), "contact_events": len(events.ContactSyncEvents), "echo_events": len(events.MessageEchoes),
	})
	// #endregion
	for i := range events.ContactSyncEvents {
		if err := s.handleContactSyncEvent(ctx, &events.ContactSyncEvents[i]); err != nil {
			return err
		}
	}
	for i := range events.HistoryEvents {
		if err := s.handleHistoryEvent(ctx, &events.HistoryEvents[i]); err != nil {
			return err
		}
	}
	for i := range events.MessageEchoes {
		if err := s.handleMessageEchoEvent(ctx, &events.MessageEchoes[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleContactSyncEvent(ctx context.Context, event *metapkg.ContactSyncEvent) error {
	ch, err := s.channels.FindByPhoneNumberID(ctx, event.PhoneNumberID)
	if err != nil {
		return nil
	}
	if event.Action == "remove" {
		return nil
	}
	now := time.Now().UTC()
	if err := s.state.EnqueueContactEvent(ctx, &ContactEvent{
		ID:          uuid.New(),
		ChannelID:   ch.ID,
		TeamID:      ch.TeamID,
		Action:      event.Action,
		PhoneNumber: event.PhoneNumber,
		FullName:    event.FullName,
		FirstName:   event.FirstName,
		CreatedAt:   now,
	}); err != nil {
		return err
	}
	result, err := s.processPendingContactEvents(ctx, ch.ID, ch.TeamID)
	if err != nil {
		return err
	}
	// #region agent log
	debug.AgentLog("H8", "channelsync/service.go:handleContactSyncEvent", "contact sync batch processed", map[string]any{
		"channel_id": ch.ID.String(), "created": result.Created, "updated": result.Updated, "duplicates": result.Duplicates,
	})
	// #endregion
	if result.Created == 0 && result.Updated == 0 && result.Duplicates == 0 {
		return nil
	}
	state, err := s.state.EnsureState(ctx, ch.ID)
	if err != nil {
		return err
	}
	state.ContactsCreated += result.Created
	state.ContactsUpdated += result.Updated
	if state.ContactsStatus != SyncCompleted {
		state.ContactsStatus = SyncCompleted
	}
	now = time.Now().UTC()
	state.ContactsLastSyncAt = &now
	state.LastSyncAt = &now
	return s.state.UpdateState(ctx, state)
}

func (s *Service) handleHistoryEvent(ctx context.Context, event *metapkg.HistoryEvent) error {
	ch, err := s.channels.FindByPhoneNumberID(ctx, event.PhoneNumberID)
	if err != nil {
		// #region agent log
		debug.AgentLog("H1", "channelsync/service.go:handleHistoryEvent", "channel not found for phone_number_id", map[string]any{
			"phone_number_id": event.PhoneNumberID, "phase": event.Phase, "progress": event.Progress, "error": err.Error(),
		})
		// #endregion
		return nil
	}

	threadCount := len(event.Threads)
	messageCount := 0
	for _, t := range event.Threads {
		messageCount += len(t.Messages)
	}
	// #region agent log
	debug.AgentLog("H3", "channelsync/service.go:handleHistoryEvent", "processing history chunk", map[string]any{
		"channel_id": ch.ID.String(), "phone_number_id": event.PhoneNumberID, "phase": event.Phase,
		"progress": event.Progress, "threads": threadCount, "messages": messageCount, "declined": event.Declined,
		"business_phone": ch.PhoneNumber,
	})
	// #endregion

	if event.Declined {
		reason := event.DeclineReason
		if reason == "" {
			reason = "No conversations available from Meta."
		}
		state, err := s.state.EnsureState(ctx, ch.ID)
		if err != nil {
			return err
		}
		state.ConversationsStatus = SyncCompleted
		state.ConversationsImported = 0
		state.ConversationsReason = &reason
		now := time.Now().UTC()
		state.ConversationsLastSyncAt = &now
		state.LastSyncAt = &now
		return s.state.UpdateState(ctx, state)
	}

	imported := 0
	businessPhone := ""
	if ch.PhoneNumber != nil {
		businessPhone = phone.Normalize(*ch.PhoneNumber)
	}
	createdMessages := 0
	skippedMessages := 0
	createErrors := 0
	for _, thread := range event.Threads {
		contactPhone := phone.Normalize(thread.ContactPhone)
		contactEntity, mergeErr := mergeContact(ctx, s.contacts, ch.TeamID, contactPhone, contactPhone, "")
		if mergeErr != nil {
			// #region agent log
			debug.AgentLog("H6", "channelsync/service.go:handleHistoryEvent", "merge contact failed", map[string]any{
				"channel_id": ch.ID.String(), "phone_number_id": event.PhoneNumberID, "contact_phone": contactPhone, "error": mergeErr.Error(),
			})
			// #endregion
			return mergeErr
		}
		conv, convErr := findOrCreateConversation(ctx, s.conversations, ch.TeamID, contactEntity.ID, ch.ID)
		if convErr != nil {
			// #region agent log
			debug.AgentLog("H6", "channelsync/service.go:handleHistoryEvent", "find/create conversation failed", map[string]any{
				"channel_id": ch.ID.String(), "phone_number_id": event.PhoneNumberID, "contact_id": contactEntity.ID.String(), "error": convErr.Error(),
			})
			// #endregion
			return convErr
		}
		if len(thread.Messages) > 0 {
			imported++
		}
		for _, histMsg := range thread.Messages {
			if histMsg.ID == "" || s.messages == nil {
				skippedMessages++
				continue
			}
			if existing, findErr := s.messages.FindByWAMID(ctx, histMsg.ID); findErr == nil && existing != nil {
				skippedMessages++
				continue
			}
			direction := message.DirectionInbound
			sender := phone.Normalize(histMsg.From)
			var recipient *string
			if businessPhone != "" && phone.Normalize(histMsg.From) == businessPhone {
				direction = message.DirectionOutbound
				recipient = &contactPhone
			}
			sentAt := metapkg.ParseUnixTimestamp(histMsg.Timestamp)
			channelID := ch.ID
			wamid := histMsg.ID
			now := time.Now().UTC()
			msg := &message.Message{
				ID:             uuid.New(),
				TeamID:         ch.TeamID,
				ConversationID: conv.ID,
				ChannelID:      &channelID,
				WAMID:          &wamid,
				Direction:      direction,
				Sender:         sender,
				Recipient:      recipient,
				Content:        histMsg.Body,
				ContentType:    message.ContentText,
				ReceivedAt:     sentAt,
				CreatedAt:      now,
			}
			if err := s.messages.Create(ctx, msg); err != nil {
				createErrors++
				// #region agent log
				debug.AgentLog("H6", "channelsync/service.go:handleHistoryEvent", "create history message failed", map[string]any{
					"channel_id": ch.ID.String(), "conversation_id": conv.ID.String(), "wamid": histMsg.ID, "error": err.Error(),
				})
				// #endregion
				return err
			}
			createdMessages++
		}
	}

	// #region agent log
	debug.AgentLog("H3", "channelsync/service.go:handleHistoryEvent", "history import result", map[string]any{
		"channel_id": ch.ID.String(), "phase": event.Phase, "progress": event.Progress,
		"threads_imported": imported, "messages_created": createdMessages, "messages_skipped": skippedMessages,
		"create_errors": createErrors, "business_phone_norm": businessPhone,
	})
	// #endregion

	state, err := s.state.EnsureState(ctx, ch.ID)
	if err != nil {
		return err
	}
	state.ConversationsImported += imported
	state.ConversationsStatus = SyncCompleted
	now := time.Now().UTC()
	state.ConversationsLastSyncAt = &now
	state.LastSyncAt = &now
	if event.Progress >= 100 {
		if imported == 0 {
			reason := "No conversations available from Meta."
			state.ConversationsReason = &reason
			// #region agent log
			debug.AgentLog("H4", "channelsync/service.go:handleHistoryEvent", "phase complete with zero threads in chunk", map[string]any{
				"channel_id": ch.ID.String(), "phase": event.Phase, "total_imported": state.ConversationsImported,
			})
			// #endregion
		}
	}
	return s.state.UpdateState(ctx, state)
}

func (s *Service) handleMessageEchoEvent(ctx context.Context, event *metapkg.MessageEchoEvent) error {
	ch, err := s.channels.FindByPhoneNumberID(ctx, event.PhoneNumberID)
	if err != nil {
		// #region agent log
		debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "channel not found for echo", map[string]any{
			"phone_number_id": event.PhoneNumberID,
		})
		// #endregion
		return nil
	}
	contactPhone := phone.Normalize(event.To)
	// #region agent log
	debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "processing echo", map[string]any{
		"channel_id": ch.ID.String(), "phone_number_id": event.PhoneNumberID, "from": event.From, "to": event.To, "to_norm": contactPhone, "wamid": event.ID,
	})
	// #endregion
	contactEntity, mergeErr := mergeContact(ctx, s.contacts, ch.TeamID, contactPhone, contactPhone, "")
	if mergeErr != nil {
		// #region agent log
		debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "merge contact failed", map[string]any{
			"channel_id": ch.ID.String(), "to_norm": contactPhone, "error": mergeErr.Error(),
		})
		// #endregion
		return mergeErr
	}
	conv, convErr := findOrCreateConversation(ctx, s.conversations, ch.TeamID, contactEntity.ID, ch.ID)
	if convErr != nil {
		// #region agent log
		debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "find/create conversation failed", map[string]any{
			"channel_id": ch.ID.String(), "contact_id": contactEntity.ID.String(), "error": convErr.Error(),
		})
		// #endregion
		return convErr
	}
	if event.ID != "" && s.messages != nil {
		if existing, findErr := s.messages.FindByWAMID(ctx, event.ID); findErr == nil && existing != nil {
			return nil
		}
	}
	sentAt := metapkg.ParseUnixTimestamp(event.Timestamp)
	channelID := ch.ID
	wamid := event.ID
	now := time.Now().UTC()
	recipient := contactPhone
	msg := &message.Message{
		ID:             uuid.New(),
		TeamID:         ch.TeamID,
		ConversationID: conv.ID,
		ChannelID:      &channelID,
		WAMID:          &wamid,
		Direction:      message.DirectionOutbound,
		Sender:         phone.Normalize(event.From),
		Recipient:      &recipient,
		Content:        event.Body,
		ContentType:    message.ContentText,
		ReceivedAt:     sentAt,
		CreatedAt:      now,
	}
	if s.messages == nil {
		// #region agent log
		debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "messages repository is nil", map[string]any{
			"channel_id": ch.ID.String(), "conversation_id": conv.ID.String(),
		})
		// #endregion
		return nil
	}
	if err := s.messages.Create(ctx, msg); err != nil {
		// #region agent log
		debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "create echo message failed", map[string]any{
			"channel_id": ch.ID.String(), "wamid": event.ID, "error": err.Error(),
		})
		// #endregion
		return err
	}
	if s.inboxSync != nil {
		_, _ = s.inboxSync.HandleOutbound(ctx, inbox.OutboundParams{
			TeamID:       ch.TeamID,
			AgentID:      uuid.Nil,
			Conversation: conv,
			Message:      msg,
		})
	}
	// #region agent log
	debug.AgentLog("H7", "channelsync/service.go:handleMessageEchoEvent", "echo message stored", map[string]any{
		"channel_id": ch.ID.String(), "conversation_id": conv.ID.String(), "wamid": event.ID, "to": contactPhone,
	})
	// #endregion
	return nil
}

func (s *Service) processPendingContactEvents(ctx context.Context, channelID, teamID uuid.UUID) (*ContactSyncResult, error) {
	events, err := s.state.ListPendingContactEvents(ctx, channelID, 5000)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	result := &ContactSyncResult{}
	processed := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		if event.Action == "remove" {
			processed = append(processed, event.ID)
			continue
		}
		outcome, _, mergeErr := mergeContactWithOutcome(ctx, s.contacts, teamID, event.PhoneNumber, event.FullName, event.FirstName)
		if mergeErr != nil {
			return nil, mergeErr
		}
		switch {
		case outcome.Created:
			result.Created++
		case outcome.Updated:
			result.Updated++
		default:
			result.Duplicates++
		}
		processed = append(processed, event.ID)
	}
	if err := s.state.MarkContactEventsProcessed(ctx, processed); err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	return result, nil
}

func (s *Service) refreshPermissions(ctx context.Context, ch *channel.Channel, token string, expired bool, state *State) (Permissions, error) {
	perms := state.Permissions
	now := time.Now().UTC()
	perms.LastCheckedAt = &now
	perms.Templates = true

	if expired || token == "" {
		perms.Contacts = false
		perms.ConversationSync = false
		state.Permissions = perms
		state.LastPermissionCheckAt = &now
		_ = s.state.UpdateState(ctx, state)
		return perms, nil
	}

	if ch.PhoneNumberID != nil && s.meta != nil {
		if coex, err := s.meta.GetCoexistenceStatus(ctx, *ch.PhoneNumberID, token); err == nil && coex != nil {
			perms.Contacts = coex.IsOnBizApp
			perms.ConversationSync = coex.IsOnBizApp
		} else if ch.IsCoex {
			perms.Contacts = true
			perms.ConversationSync = true
		}
		if ch.WABAID != nil {
			if _, _, err := s.meta.ListMessageTemplatesPage(ctx, *ch.WABAID, token, ""); err == nil {
				perms.Templates = true
			}
		}
	} else if ch.IsCoex {
		perms.Contacts = true
		perms.ConversationSync = true
	}

	state.Permissions = perms
	state.LastPermissionCheckAt = &now
	if err := s.state.UpdateState(ctx, state); err != nil {
		return perms, serviceerrors.MapRepository(err)
	}
	return perms, nil
}

func (s *Service) loadChannelToken(ctx context.Context, teamID, channelID uuid.UUID) (*channel.Channel, string, bool, error) {
	ch, err := s.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return nil, "", false, serviceerrors.MapRepository(err)
	}
	if ch.Status != channel.StatusConnected {
		return nil, "", false, serviceerrors.NewValidation("channel", "channel is not connected")
	}
	creds, err := s.channels.FindCredentials(ctx, channelID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, "", false, serviceerrors.NewValidation("channel", "channel credentials are missing")
		}
		return nil, "", false, serviceerrors.MapRepository(err)
	}
	if s.cipher == nil {
		return nil, "", false, errors.New("token cipher is not configured")
	}
	token, err := s.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return nil, "", false, err
	}
	expired := false
	if creds.TokenExpiresAt != nil && creds.TokenExpiresAt.Before(time.Now().UTC()) {
		expired = true
	}
	return ch, token, expired, nil
}

func (s *Service) authorize(actorRole string) error {
	if s.rbac != nil && s.rbac.Require(actorRole, rbac.PermDeviceManage) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	if err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (s *Service) refreshReadiness(ctx context.Context, teamID uuid.UUID) {
	if s.readiness == nil {
		return
	}
	_ = s.readiness.RefreshReadinessInternal(ctx, teamID)
}

type mergeOutcome struct {
	Created bool
	Updated bool
}

func mergeContact(ctx context.Context, contacts contact.Repository, teamID uuid.UUID, rawPhone, displayName, profileName string) (*contact.Contact, error) {
	outcome, c, err := mergeContactWithOutcome(ctx, contacts, teamID, rawPhone, displayName, profileName)
	if err != nil {
		return nil, err
	}
	_ = outcome
	return c, nil
}

func mergeContactWithOutcome(ctx context.Context, contacts contact.Repository, teamID uuid.UUID, rawPhone, displayName, profileName string) (mergeOutcome, *contact.Contact, error) {
	phoneNorm := phone.Normalize(rawPhone)
	name := pickDisplayName(displayName, profileName, phoneNorm)
	phonePlus := ""
	if phoneNorm != "" {
		phonePlus = "+" + phoneNorm
	}

	existing, err := contacts.FindByPhone(ctx, teamID, phoneNorm)
	if err != nil && phonePlus != "" {
		if alt, altErr := contacts.FindByPhone(ctx, teamID, phonePlus); altErr == nil {
			existing = alt
			err = nil
		}
	}
	if err == nil {
		updated := false
		customerName := strings.TrimSpace(profileName)
		if customerName == "" {
			customerName = strings.TrimSpace(name)
		}
		if customerName != "" && customerName != existing.CustomerName {
			existing.CustomerName = customerName
			updated = true
		}
		if name != "" && contact.ShouldAutoUpdateContactName(existing) && name != existing.ContactName {
			existing.ContactName = name
			updated = true
		}
		if updated {
			existing.UpdatedAt = time.Now().UTC()
			if err := contacts.Update(ctx, existing); err != nil {
				return mergeOutcome{}, nil, err
			}
			return mergeOutcome{Updated: true}, existing, nil
		}
		return mergeOutcome{}, existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return mergeOutcome{}, nil, err
	}

	now := time.Now().UTC()
	storePhone := phonePlus
	if storePhone == "" {
		storePhone = phoneNorm
	}
	inboxName := strings.TrimSpace(name)
	customerName := strings.TrimSpace(profileName)
	if customerName == "" {
		customerName = inboxName
	}
	c := &contact.Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		ContactName:     inboxName,
		CustomerName:    customerName,
		Phone:           storePhone,
		Source:          contact.SourceMetaInbound,
		MarketingStatus: contact.MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := contacts.Create(ctx, c); err != nil {
		return mergeOutcome{}, nil, err
	}
	return mergeOutcome{Created: true}, c, nil
}

func pickDisplayName(displayName, profileName, fallback string) string {
	if name := strings.TrimSpace(displayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(profileName); name != "" {
		return name
	}
	return fallback
}

func findOrCreateConversation(ctx context.Context, convos conversation.Repository, teamID, contactID, channelID uuid.UUID) (*conversation.Conversation, error) {
	channelPtr := &channelID
	existing, err := convos.FindByContactChannel(ctx, teamID, contactID, channelPtr)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	now := time.Now().UTC()
	conv := &conversation.Conversation{
		ID:          uuid.New(),
		TeamID:      teamID,
		ContactID:   contactID,
		ChannelID:   channelPtr,
		ChannelType: conversation.ChannelWhatsApp,
		Status:      conversation.StatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := convos.Create(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}
