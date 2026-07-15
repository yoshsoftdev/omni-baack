package channelmgmt

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Supported channel type identifiers. Future types may be added as unsupported.
const (
	TypeWhatsApp  = "whatsapp"
	TypeInstagram = "instagram"
	TypeMessenger = "messenger"
	TypeTelegram  = "telegram"
	TypeEmail     = "email"
)

// ChannelStore reads workspace channels without mutating connection state.
type ChannelStore interface {
	List(ctx context.Context, teamID uuid.UUID, filter channel.ListFilter, page repository.PageParams) (repository.Page[channel.Channel], error)
	FindConnectedByType(ctx context.Context, teamID uuid.UUID, channelType channel.ChannelType) (*channel.Channel, error)
}

// Service reports supported channel types and connection status.
// It does not perform OAuth or call Meta.
type Service struct {
	channels ChannelStore
	teams    team.Repository
	rbac     *rbac.Service
}

func NewService(channels ChannelStore, teams team.Repository, rbacSvc *rbac.Service) *Service {
	return &Service{channels: channels, teams: teams, rbac: rbacSvc}
}

// ChannelTypeInfo describes a platform capability.
type ChannelTypeInfo struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Supported   bool   `json:"supported"`
}

// ChannelStatusEntry is the connection status for one channel type.
type ChannelStatusEntry struct {
	Type        string `json:"type"`
	Connected   bool   `json:"connected"`
	DisplayName string `json:"display_name,omitempty"`
}

// StatusReport is the workspace channel connection snapshot.
type StatusReport struct {
	Channels []ChannelStatusEntry `json:"channels"`
}

// InstagramStatus is Instagram-only connection state.
type InstagramStatus struct {
	Connected   bool   `json:"connected"`
	DisplayName string `json:"display_name,omitempty"`
}

func (s *Service) authorize(ctx context.Context, teamID, userID uuid.UUID, actorRole string) error {
	if s.rbac != nil && !s.rbac.Require(actorRole, rbac.PermDeviceView) {
		return serviceerrors.ErrForbidden
	}
	if s.teams == nil {
		return nil
	}
	if _, err := s.teams.FindMemberByUser(ctx, teamID, userID); err != nil {
		if err == team.ErrMemberNotFound {
			return serviceerrors.ErrForbidden
		}
		return err
	}
	return nil
}

// ListTypes returns supported channel types.
func (s *Service) ListTypes(ctx context.Context, teamID, userID uuid.UUID, actorRole string) ([]ChannelTypeInfo, error) {
	if err := s.authorize(ctx, teamID, userID, actorRole); err != nil {
		return nil, err
	}
	return []ChannelTypeInfo{
		{Type: TypeWhatsApp, DisplayName: "WhatsApp Business", Supported: true},
		{Type: TypeInstagram, DisplayName: "Instagram Business", Supported: true},
	}, nil
}

// Status reports connection state for supported channel types.
func (s *Service) Status(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*StatusReport, error) {
	if err := s.authorize(ctx, teamID, userID, actorRole); err != nil {
		return nil, err
	}

	whatsapp := ChannelStatusEntry{Type: TypeWhatsApp, Connected: false}
	instagram := ChannelStatusEntry{Type: TypeInstagram, Connected: false}

	if s.channels != nil {
		waType := channel.TypeWhatsApp
		page, err := s.channels.List(ctx, teamID, channel.ListFilter{ChannelType: &waType}, repository.PageParams{Page: 1, PageSize: 100})
		if err != nil {
			return nil, serviceerrors.MapRepository(err)
		}
		for _, ch := range page.Items {
			if ch.Status == channel.StatusConnected {
				whatsapp.Connected = true
				whatsapp.DisplayName = ch.Name
				break
			}
		}
		if !whatsapp.Connected {
			for _, ch := range page.Items {
				if ch.Name != "" {
					whatsapp.DisplayName = ch.Name
					break
				}
			}
		}

		ig, err := s.channels.FindConnectedByType(ctx, teamID, channel.TypeInstagram)
		if err == nil && ig != nil {
			instagram.Connected = true
			instagram.DisplayName = ig.Name
		} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, serviceerrors.MapRepository(err)
		}
	}

	return &StatusReport{Channels: []ChannelStatusEntry{whatsapp, instagram}}, nil
}

// InstagramStatus reports Instagram connection state from the database.
func (s *Service) InstagramStatus(ctx context.Context, teamID, userID uuid.UUID, actorRole string) (*InstagramStatus, error) {
	if err := s.authorize(ctx, teamID, userID, actorRole); err != nil {
		return nil, err
	}
	if s.channels == nil {
		return &InstagramStatus{Connected: false}, nil
	}
	ig, err := s.channels.FindConnectedByType(ctx, teamID, channel.TypeInstagram)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &InstagramStatus{Connected: false}, nil
		}
		return nil, serviceerrors.MapRepository(err)
	}
	return &InstagramStatus{Connected: true, DisplayName: ig.Name}, nil
}
