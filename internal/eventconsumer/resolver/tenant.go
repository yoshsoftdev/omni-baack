package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/consumererrors"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// TenantContext holds resolved team and channel for an event.
type TenantContext struct {
	TeamID    uuid.UUID
	ChannelID uuid.UUID
	Channel   *channel.Channel
}

// TenantResolver resolves phone_number_id to channel and team.
type TenantResolver struct {
	channels channel.Repository
}

func NewTenantResolver(channels channel.Repository) *TenantResolver {
	return &TenantResolver{channels: channels}
}

func (r *TenantResolver) ResolveByPhoneNumberID(ctx context.Context, phoneNumberID string) (*TenantContext, error) {
	phoneNumberID = strings.TrimSpace(phoneNumberID)
	if phoneNumberID == "" {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	return toTenant(r.channels.FindByPhoneNumberID(ctx, phoneNumberID))
}

func (r *TenantResolver) ResolveByWABAID(ctx context.Context, wabaID string) (*TenantContext, error) {
	wabaID = strings.TrimSpace(wabaID)
	if wabaID == "" {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	return toTenant(r.channels.FindByWABAID(ctx, wabaID))
}

func (r *TenantResolver) ResolveByIGUserID(ctx context.Context, igUserID string) (*TenantContext, error) {
	igUserID = strings.TrimSpace(igUserID)
	if igUserID == "" {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	return toTenant(r.channels.FindByIGUserIDGlobal(ctx, igUserID))
}

// toTenant dead-letters only a channel that does not exist. Any other lookup
// error (DB down, timeout) is returned as retryable so the event is redelivered
// instead of being dropped to the DLQ during an outage.
func toTenant(ch *channel.Channel, err error) (*TenantContext, error) {
	if errors.Is(err, repository.ErrNotFound) {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve channel: %w", err)
	}
	return &TenantContext{TeamID: ch.TeamID, ChannelID: ch.ID, Channel: ch}, nil
}

func terminalDLQ(category string, err error) error {
	return consumererrors.TerminalDLQ(category, err)
}
