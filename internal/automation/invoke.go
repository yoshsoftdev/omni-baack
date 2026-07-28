package automation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type MetaSender interface {
	SendTemplateMessage(
		ctx context.Context,
		phoneNumberID, accessToken, to, templateName, language string,
		components []metaclient.TemplateComponent,
	) (string, error)
	SendTextMessage(
		ctx context.Context,
		phoneNumberID, accessToken, to, body string,
	) (string, error)
}

type ChannelStore interface {
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*channel.Channel, error)
	FindCredentials(ctx context.Context, channelID uuid.UUID) (*channel.Credentials, error)
}

type ContactStore interface {
	FindByPhone(ctx context.Context, teamID uuid.UUID, phone string) (*contact.Contact, error)
	Create(ctx context.Context, c *contact.Contact) error
}

type ConversationStore interface {
	FindByContactChannel(ctx context.Context, teamID, contactID uuid.UUID, channelID *uuid.UUID) (*conversation.Conversation, error)
	Create(ctx context.Context, c *conversation.Conversation) error
	UpdateLastMessage(ctx context.Context, teamID, id uuid.UUID, update conversation.LastMessageUpdate) error
}

type MessageStore interface {
	ListByConversation(ctx context.Context, teamID, conversationID uuid.UUID, filter message.ListFilter, page repository.PageParams) (repository.Page[message.Message], error)
	Create(ctx context.Context, m *message.Message) error
}

type Invoker struct {
	repo          Repository
	templates     TemplateFinder
	channels      ChannelStore
	contacts      ContactStore
	conversations ConversationStore
	messages      MessageStore
	cipher        *cryptoutil.TokenCipher
	meta          MetaSender
	httpClient    *http.Client
}

type invokeStatusError struct {
	statusCode int
	message    string
}

func (e *invokeStatusError) Error() string { return e.message }

func NewInvoker(
	repo Repository,
	templates TemplateFinder,
	channels ChannelStore,
	contacts ContactStore,
	conversations ConversationStore,
	messages MessageStore,
	cipher *cryptoutil.TokenCipher,
	meta MetaSender,
) *Invoker {
	return &Invoker{
		repo:          repo,
		templates:     templates,
		channels:      channels,
		contacts:      contacts,
		conversations: conversations,
		messages:      messages,
		cipher:        cipher,
		meta:          meta,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (i *Invoker) AuthenticateAPIKey(ctx context.Context, bearer string) (*APIKey, error) {
	token := strings.TrimSpace(bearer)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, serviceerrors.ErrForbidden
	}
	key, err := i.repo.FindAPIKeyByHash(ctx, HashAPIKey(token))
	if err != nil {
		return nil, serviceerrors.ErrForbidden
	}
	if key.Status != APIKeyStatusActive {
		return nil, serviceerrors.ErrForbidden
	}
	return key, nil
}

func (i *Invoker) InvokePublic(ctx context.Context, endpointID, bearer string, body map[string]any, clientIP string) (*InvokeResult, error) {
	key, err := i.AuthenticateAPIKey(ctx, bearer)
	if err != nil {
		return nil, err
	}
	ep, err := i.repo.FindEndpointByIDGlobal(ctx, endpointID)
	if err != nil {
		return nil, serviceerrors.MapRepository(err)
	}
	if ep.TeamID != key.TeamID {
		return nil, serviceerrors.ErrNotFound
	}
	hasScope := false
	for _, scope := range key.Scopes {
		if scope == "send_template" || scope == "send_text" || scope == "endpoints.invoke" {
			hasScope = true
			break
		}
	}
	if !hasScope {
		return nil, serviceerrors.ErrForbidden
	}
	return i.Invoke(ctx, ep, key, body, clientIP)
}

func (i *Invoker) Invoke(ctx context.Context, ep *Endpoint, key *APIKey, body map[string]any, clientIP string) (*InvokeResult, error) {
	start := time.Now()
	execID := NewExecutionID()
	now := time.Now().UTC()
	var handled *InvokeResult

	fail := func(statusCode int, message string) (*InvokeResult, error) {
		msg := message
		var keyID *string
		if key != nil {
			keyID = &key.ID
		}
		var ip *string
		if clientIP != "" {
			ip = &clientIP
		}
		exec := &Execution{
			ID:           execID,
			TeamID:       ep.TeamID,
			EndpointID:   ep.ID,
			APIKeyID:     keyID,
			Status:       ExecutionFailed,
			StatusCode:   statusCode,
			LatencyMs:    int(time.Since(start).Milliseconds()),
			ErrorMessage: &msg,
			ClientIP:     ip,
			CreatedAt:    now,
		}
		_ = i.repo.CreateExecution(ctx, exec)
		handled = &InvokeResult{
			Success:     false,
			StatusCode:  statusCode,
			LatencyMs:   int(time.Since(start).Milliseconds()),
			ExecutionID: execID,
			Body:        map[string]any{"success": false, "message": message},
		}
		i.dispatchEndpointWebhook(ctx, ep, exec, handled.Body)
		return handled, nil
	}

	if ep.Status != EndpointStatusActive {
		return fail(403, "Endpoint is disabled")
	}
	if i.meta == nil || i.channels == nil || i.cipher == nil {
		return fail(503, "Automation invoke is not configured")
	}

	wamid, err := i.invokeByAction(ctx, ep, body, fail)
	if handled != nil {
		return handled, nil
	}
	latency := int(time.Since(start).Milliseconds())
	var keyID *string
	if key != nil {
		keyID = &key.ID
	}
	var ip *string
	if clientIP != "" {
		ip = &clientIP
	}

	if err != nil {
		msg := err.Error()
		exec := &Execution{
			ID:           execID,
			TeamID:       ep.TeamID,
			EndpointID:   ep.ID,
			APIKeyID:     keyID,
			Status:       ExecutionFailed,
			StatusCode:   502,
			LatencyMs:    latency,
			ErrorMessage: &msg,
			ClientIP:     ip,
			CreatedAt:    now,
		}
		_ = i.repo.CreateExecution(ctx, exec)
		out := &InvokeResult{
			Success:     false,
			StatusCode:  502,
			LatencyMs:   latency,
			ExecutionID: execID,
			Body:        map[string]any{"success": false, "message": "Failed to send template"},
		}
		i.dispatchEndpointWebhook(ctx, ep, exec, out.Body)
		return out, nil
	}

	exec := &Execution{
		ID:                execID,
		TeamID:            ep.TeamID,
		EndpointID:        ep.ID,
		APIKeyID:          keyID,
		Status:            ExecutionSuccess,
		StatusCode:        200,
		LatencyMs:         latency,
		ClientIP:          ip,
		ProviderMessageID: &wamid,
		CreatedAt:         now,
	}
	_ = i.repo.CreateExecution(ctx, exec)
	_ = i.repo.TouchEndpointUsed(ctx, ep.ID, now)
	if key != nil {
		_ = i.repo.TouchAPIKeyUsed(ctx, key.ID, now)
	}

	out := &InvokeResult{
		Success:     true,
		StatusCode:  200,
		LatencyMs:   latency,
		ExecutionID: execID,
		MessageID:   wamid,
		Body: map[string]any{
			"success": true,
			"data": map[string]any{
				"executionId": execID,
				"messageId":   wamid,
				"status":      "queued",
			},
		},
	}
	i.dispatchEndpointWebhook(ctx, ep, exec, out.Body)
	return out, nil
}

func (i *Invoker) invokeByAction(
	ctx context.Context,
	ep *Endpoint,
	body map[string]any,
	fail func(statusCode int, message string) (*InvokeResult, error),
) (string, error) {
	switch ep.ActionType {
	case ActionSendTemplate:
		if i.templates == nil {
			_, err := fail(503, "Automation invoke is not configured")
			return "", err
		}
		return i.invokeSendTemplate(ctx, ep, body, fail)
	case ActionSendText:
		if i.contacts == nil || i.conversations == nil || i.messages == nil {
			_, err := fail(503, "Automation invoke is not configured")
			return "", err
		}
		return i.invokeSendText(ctx, ep, body, fail)
	default:
		_, err := fail(422, "Unsupported action type")
		return "", err
	}
}

func (i *Invoker) invokeSendTemplate(
	ctx context.Context,
	ep *Endpoint,
	body map[string]any,
	fail func(statusCode int, message string) (*InvokeResult, error),
) (string, error) {
	cfg, err := parseAndValidateSendTemplateConfig(ep.ActionConfig)
	if err != nil {
		_, errFail := fail(422, err.Error())
		return "", errFail
	}

	toRaw, _ := body["to"].(string)
	toRaw = strings.TrimSpace(toRaw)
	if toRaw == "" {
		_, errFail := fail(422, "Recipient phone number (to) is required")
		return "", errFail
	}
	digits := phone.Normalize(toRaw)
	if digits == "" {
		_, errFail := fail(422, "Recipient phone number is invalid")
		return "", errFail
	}
	storedPhone := "+" + digits
	to := digits

	variables := map[string]string{}
	mapping := campaign.VariableMapping{Body: map[string]string{}}
	for _, m := range cfg.VariableMappings {
		val := ""
		if raw, ok := body[m.APIParameter]; ok && raw != nil {
			val = strings.TrimSpace(fmt.Sprint(raw))
		}
		if val == "" {
			val = strings.TrimSpace(m.DefaultValue)
		}
		if val == "" && m.Required {
			_, errFail := fail(422, fmt.Sprintf("Missing required parameter %q", m.APIParameter))
			return "", errFail
		}
		mapping.Body[m.TemplateVariable] = m.APIParameter
		variables["body."+m.TemplateVariable] = val
	}

	tplID, err := uuid.Parse(cfg.TemplateID)
	if err != nil {
		_, errFail := fail(422, "Invalid template id")
		return "", errFail
	}
	tpl, err := i.templates.FindByID(ctx, ep.TeamID, tplID)
	if err != nil {
		_, errFail := fail(404, "Template not found")
		return "", errFail
	}
	if tpl.Status != template.StatusApproved {
		_, errFail := fail(422, "Template is not approved")
		return "", errFail
	}

	phoneNumberID, token, accessErr := i.resolveChannelAccessFromTemplate(ctx, ep.TeamID, tpl.ChannelID)
	if accessErr != nil {
		typed := accessErr.(*invokeStatusError)
		_, errFail := fail(typed.statusCode, typed.message)
		return "", errFail
	}

	components, err := campaign.RenderValidatedTemplate(tpl, mapping, variables)
	if err != nil {
		_, errFail := fail(422, err.Error())
		return "", errFail
	}

	wamid, err := i.meta.SendTemplateMessage(
		ctx,
		phoneNumberID,
		token,
		to,
		tpl.Name,
		template.ToMetaSendLanguage(tpl.Language),
		components,
	)
	if err != nil {
		return "", err
	}
	contactEntity, histErr := i.ensureContactByPhone(ctx, ep.TeamID, storedPhone)
	if histErr != nil {
		return "", histErr
	}
	conv, histErr := i.ensureConversation(ctx, ep.TeamID, contactEntity.ID, tpl.ChannelID)
	if histErr != nil {
		return "", histErr
	}
	preview := "Template: " + strings.TrimSpace(tpl.Name)
	if histErr := i.recordOutboundHistory(ctx, ep, conv, storedPhone, preview, message.ContentTemplate, &wamid); histErr != nil {
		return "", histErr
	}
	return wamid, nil
}

func (i *Invoker) invokeSendText(
	ctx context.Context,
	ep *Endpoint,
	body map[string]any,
	fail func(statusCode int, message string) (*InvokeResult, error),
) (string, error) {
	cfg, err := parseAndValidateSendTextConfig(ep.ActionConfig)
	if err != nil {
		_, errFail := fail(422, err.Error())
		return "", errFail
	}
	channelID, err := uuid.Parse(cfg.ChannelID)
	if err != nil {
		_, errFail := fail(422, "Invalid channel id")
		return "", errFail
	}
	toRaw, _ := body["to"].(string)
	toRaw = strings.TrimSpace(toRaw)
	if toRaw == "" {
		_, errFail := fail(422, "Recipient phone number (to) is required")
		return "", errFail
	}

	// Contact phone is stored with '+' prefix (see contact normalizeInboundPhone).
	// Meta "to" field must be digits-only, so keep both representations.
	digits := phone.Normalize(toRaw)
	if digits == "" {
		_, errFail := fail(422, "Recipient phone number is invalid")
		return "", errFail
	}
	storedPhone := "+" + digits
	to := digits

	param := cfg.TextParameter
	if param == "" {
		param = "text"
	}
	text := ""
	if raw, ok := body[param]; ok && raw != nil {
		text = strings.TrimSpace(fmt.Sprint(raw))
	}
	if text == "" {
		text = strings.TrimSpace(cfg.DefaultText)
	}
	if text == "" {
		_, errFail := fail(422, fmt.Sprintf("Text parameter %q is required", param))
		return "", errFail
	}

	contactEntity, err := i.contacts.FindByPhone(ctx, ep.TeamID, storedPhone)
	if err != nil {
		_, errFail := fail(404, "Contact not found for recipient")
		return "", errFail
	}
	conv, err := i.conversations.FindByContactChannel(ctx, ep.TeamID, contactEntity.ID, &channelID)
	if err != nil {
		_, errFail := fail(404, "Conversation not found for recipient and channel")
		return "", errFail
	}
	if conv.Status != conversation.StatusOpen && conv.Status != conversation.StatusPending {
		_, errFail := fail(422, "Conversation is not open")
		return "", errFail
	}

	inboundDir := message.DirectionInbound
	msgPage, err := i.messages.ListByConversation(
		ctx,
		ep.TeamID,
		conv.ID,
		message.ListFilter{Direction: &inboundDir},
		repository.PageParams{Page: 1, PageSize: 1},
	)
	if err != nil || len(msgPage.Items) == 0 {
		_, errFail := fail(422, "No inbound message found in conversation")
		return "", errFail
	}
	lastInbound := msgPage.Items[0]
	if time.Since(lastInbound.CreatedAt) > 24*time.Hour {
		_, errFail := fail(422, "Free text is allowed only within 24h of last inbound message")
		return "", errFail
	}

	phoneNumberID, token, accessErr := i.resolveChannelAccessFromConfig(ctx, ep.TeamID, channelID)
	if accessErr != nil {
		typed := accessErr.(*invokeStatusError)
		_, errFail := fail(typed.statusCode, typed.message)
		return "", errFail
	}

	wamid, err := i.meta.SendTextMessage(ctx, phoneNumberID, token, to, text)
	if err != nil {
		return "", err
	}
	if histErr := i.recordOutboundHistory(ctx, ep, conv, storedPhone, text, message.ContentText, &wamid); histErr != nil {
		return "", histErr
	}
	return wamid, nil
}

func (i *Invoker) resolveChannelAccessFromTemplate(
	ctx context.Context,
	teamID uuid.UUID,
	channelID uuid.UUID,
) (string, string, error) {
	ch, err := i.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel not found for template"}
	}
	if ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel missing phone_number_id"}
	}
	creds, err := i.channels.FindCredentials(ctx, ch.ID)
	if err != nil {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel credentials not found"}
	}
	token, err := i.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return "", "", &invokeStatusError{statusCode: 500, message: "Failed to decrypt channel token"}
	}
	return strings.TrimSpace(*ch.PhoneNumberID), token, nil
}

func (i *Invoker) resolveChannelAccessFromConfig(
	ctx context.Context,
	teamID uuid.UUID,
	channelID uuid.UUID,
) (string, string, error) {
	ch, err := i.channels.FindByID(ctx, teamID, channelID)
	if err != nil {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel not found"}
	}
	if ch.ChannelType != channel.TypeWhatsApp {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel must be WhatsApp"}
	}
	if ch.Status != channel.StatusConnected {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel is not connected"}
	}
	if ch.PhoneNumberID == nil || strings.TrimSpace(*ch.PhoneNumberID) == "" {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel missing phone_number_id"}
	}
	creds, err := i.channels.FindCredentials(ctx, ch.ID)
	if err != nil {
		return "", "", &invokeStatusError{statusCode: 422, message: "Channel credentials not found"}
	}
	token, err := i.cipher.Decrypt(creds.AccessTokenEnc)
	if err != nil {
		return "", "", &invokeStatusError{statusCode: 500, message: "Failed to decrypt channel token"}
	}
	return strings.TrimSpace(*ch.PhoneNumberID), token, nil
}

func (i *Invoker) dispatchEndpointWebhook(ctx context.Context, ep *Endpoint, exec *Execution, responseBody map[string]any) {
	if ep == nil || exec == nil {
		return
	}
	url := strings.TrimSpace(ep.Webhook.URL)
	if url == "" || !ep.Webhook.IsEnabled || i.httpClient == nil {
		return
	}

	payload := map[string]any{
		"endpointId":   ep.ID,
		"executionId":  exec.ID,
		"status":       exec.Status,
		"statusCode":   exec.StatusCode,
		"latencyMs":    exec.LatencyMs,
		"messageId":    exec.ProviderMessageID,
		"errorMessage": exec.ErrorMessage,
		"occurredAt":   exec.CreatedAt,
		"response":     responseBody,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		i.recordWebhookDelivery(ctx, ep, exec, 0, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Omni-Automation-Event", "automation.execution")
	req.Header.Set("X-Omni-Automation-Execution-Id", exec.ID)
	if secret := strings.TrimSpace(ep.Webhook.Secret); secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Omni-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := i.httpClient.Do(req)
	if err != nil {
		i.recordWebhookDelivery(ctx, ep, exec, 0, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		i.recordWebhookDelivery(ctx, ep, exec, resp.StatusCode, "")
		return
	}
	i.recordWebhookDelivery(ctx, ep, exec, resp.StatusCode, "non-2xx webhook response")
}

func (i *Invoker) recordWebhookDelivery(ctx context.Context, ep *Endpoint, exec *Execution, statusCode int, errMsg string) {
	status := WebhookDeliverySuccess
	var errPtr *string
	if strings.TrimSpace(errMsg) != "" || statusCode >= 400 || statusCode == 0 {
		status = WebhookDeliveryFailed
		msg := strings.TrimSpace(errMsg)
		if msg == "" {
			msg = "webhook delivery failed"
		}
		errPtr = &msg
	}
	_ = i.repo.CreateWebhookDelivery(ctx, &WebhookDelivery{
		ID:           NewWebhookDeliveryID(),
		TeamID:       ep.TeamID,
		EndpointID:   ep.ID,
		ExecutionID:  exec.ID,
		Status:       status,
		StatusCode:   statusCode,
		ErrorMessage: errPtr,
		AttemptedAt:  time.Now().UTC(),
	})
}

func (i *Invoker) ensureContactByPhone(ctx context.Context, teamID uuid.UUID, phoneWithPlus string) (*contact.Contact, error) {
	c, err := i.contacts.FindByPhone(ctx, teamID, phoneWithPlus)
	if err == nil {
		return c, nil
	}
	if err != repository.ErrNotFound {
		return nil, err
	}
	now := time.Now().UTC()
	c = &contact.Contact{
		ID:              uuid.New(),
		TeamID:          teamID,
		ContactName:     phoneWithPlus,
		CustomerName:    phoneWithPlus,
		Phone:           phoneWithPlus,
		Source:          contact.SourceAPI,
		MarketingStatus: contact.MarketingUnknown,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := i.contacts.Create(ctx, c); err != nil {
		// Handle racing creates by re-reading.
		existing, findErr := i.contacts.FindByPhone(ctx, teamID, phoneWithPlus)
		if findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return c, nil
}

func (i *Invoker) ensureConversation(ctx context.Context, teamID, contactID, channelID uuid.UUID) (*conversation.Conversation, error) {
	channelPtr := &channelID
	conv, err := i.conversations.FindByContactChannel(ctx, teamID, contactID, channelPtr)
	if err == nil {
		return conv, nil
	}
	if err != repository.ErrNotFound {
		return nil, err
	}
	now := time.Now().UTC()
	conv = &conversation.Conversation{
		ID:          uuid.New(),
		TeamID:      teamID,
		ContactID:   contactID,
		ChannelID:   channelPtr,
		ChannelType: conversation.ChannelWhatsApp,
		Status:      conversation.StatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := i.conversations.Create(ctx, conv); err != nil {
		existing, findErr := i.conversations.FindByContactChannel(ctx, teamID, contactID, channelPtr)
		if findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return conv, nil
}

func (i *Invoker) recordOutboundHistory(
	ctx context.Context,
	ep *Endpoint,
	conv *conversation.Conversation,
	recipient string,
	content string,
	contentType message.ContentType,
	wamid *string,
) error {
	now := time.Now().UTC()
	sent := message.StatusSent
	msg := &message.Message{
		ID:             uuid.New(),
		ConversationID: conv.ID,
		TeamID:         ep.TeamID,
		ChannelID:      conv.ChannelID,
		Direction:      message.DirectionOutbound,
		Sender:         "automation:" + ep.ID,
		Recipient:      &recipient,
		Content:        strings.TrimSpace(content),
		ContentType:    contentType,
		WAMID:          wamid,
		Status:         &sent,
		ReceivedAt:     now,
		CreatedAt:      now,
	}
	if msg.Content == "" {
		msg.Content = "Automated message"
	}
	if err := i.messages.Create(ctx, msg); err != nil {
		return err
	}
	preview := msg.Content
	if len(preview) > 200 {
		preview = preview[:200]
	}
	return i.conversations.UpdateLastMessage(ctx, ep.TeamID, conv.ID, conversation.LastMessageUpdate{
		Preview: preview,
		At:      now,
	})
}