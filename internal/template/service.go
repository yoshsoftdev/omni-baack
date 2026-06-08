package template

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

var (
	ErrForbidden          = errors.New("insufficient permissions")
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDeviceNotReady     = errors.New("device is not configured for templates")
	ErrDeviceIDRequired   = errors.New("device_id is required")
)

type Service struct {
	templates Repository
	devices   device.Repository
	teams     team.Repository
	meta      *metaclient.Client
}

func NewService(
	templates Repository,
	devices device.Repository,
	teams team.Repository,
	meta *metaclient.Client,
) *Service {
	return &Service{
		templates: templates,
		devices:   devices,
		teams:     teams,
		meta:      meta,
	}
}

type SyncInput struct {
	DeviceID uuid.UUID
}

type SyncResult struct {
	Synced int `json:"synced"`
}

func (s *Service) List(ctx context.Context, teamID, userID uuid.UUID, deviceID *uuid.UUID) ([]Response, error) {
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	items, err := s.templates.ListByTeam(ctx, teamID, deviceID)
	if err != nil {
		return nil, err
	}
	responses := make([]Response, 0, len(items))
	for i := range items {
		responses = append(responses, ToResponse(&items[i]))
	}
	return responses, nil
}

func (s *Service) Sync(ctx context.Context, teamID, userID uuid.UUID, role team.Role, input SyncInput) (*SyncResult, error) {
	if !canManage(role) {
		return nil, ErrForbidden
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	if input.DeviceID == uuid.Nil {
		return nil, ErrDeviceIDRequired
	}

	dev, err := s.devices.FindByID(ctx, teamID, input.DeviceID)
	if err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	if dev.WabaID == nil || dev.AccessToken == nil || strings.TrimSpace(*dev.WabaID) == "" || strings.TrimSpace(*dev.AccessToken) == "" {
		return nil, ErrDeviceNotReady
	}

	metaTemplates, err := s.meta.ListMessageTemplates(ctx, *dev.WabaID, *dev.AccessToken)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	synced := 0
	for _, mt := range metaTemplates {
		components := mt.Components
		if len(components) == 0 {
			components = json.RawMessage("[]")
		}
		category := strings.TrimSpace(mt.Category)
		status := strings.TrimSpace(mt.Status)
		var categoryPtr *string
		var statusPtr *string
		if category != "" {
			categoryPtr = &category
		}
		if status != "" {
			statusPtr = &status
		}
		tpl := &Template{
			ID:             uuid.New(),
			TeamID:         teamID,
			DeviceID:       dev.ID,
			WabaID:         *dev.WabaID,
			MetaTemplateID: mt.ID,
			Name:           mt.Name,
			Language:       mt.Language,
			Category:       categoryPtr,
			Status:         statusPtr,
			Components:     components,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.templates.Upsert(ctx, tpl); err != nil {
			return nil, err
		}
		synced++
	}

	return &SyncResult{Synced: synced}, nil
}

func (s *Service) ensureMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.teams.FindMemberByUser(ctx, teamID, userID)
	return err
}

func canManage(role team.Role) bool {
	return role == team.RoleOwner || role == team.RoleAdmin
}
