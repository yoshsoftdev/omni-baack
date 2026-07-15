package resolver

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/consumererrors"
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
	ch, err := r.channels.FindByPhoneNumberID(ctx, phoneNumberID)
	if err != nil {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	return &TenantContext{
		TeamID:    ch.TeamID,
		ChannelID: ch.ID,
		Channel:   ch,
	}, nil
}

func (r *TenantResolver) ResolveByWABAID(ctx context.Context, wabaID string) (*TenantContext, error) {
	wabaID = strings.TrimSpace(wabaID)
	if wabaID == "" {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	ch, err := r.channels.FindByWABAID(ctx, wabaID)
	if err != nil {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	return &TenantContext{
		TeamID:    ch.TeamID,
		ChannelID: ch.ID,
		Channel:   ch,
	}, nil
}

func (r *TenantResolver) ResolveByIGUserID(ctx context.Context, igUserID string) (*TenantContext, error) {
	igUserID = strings.TrimSpace(igUserID)
	if igUserID == "" {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	ch, err := r.channels.FindByIGUserIDGlobal(ctx, igUserID)
	if err != nil {
		return nil, terminalDLQ("channel", consumererrors.ErrUnknownChannel)
	}
	return &TenantContext{
		TeamID:    ch.TeamID,
		ChannelID: ch.ID,
		Channel:   ch,
	}, nil
}

func terminalDLQ(category string, err error) error {
	return consumererrors.TerminalDLQ(category, err)
}