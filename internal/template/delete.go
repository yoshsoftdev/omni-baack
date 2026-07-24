package template

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Delete removes a template from Meta, then soft-deletes the local row.
func (s *Service) Delete(ctx context.Context, teamID, userID uuid.UUID, actorRole string, templateID uuid.UUID) error {
	if err := s.authorize(actorRole, rbac.PermTemplateCreate); err != nil {
		return err
	}
	if err := s.ensureMember(ctx, teamID, userID); err != nil {
		return err
	}
	if s.sync == nil {
		return serviceerrors.NewValidation("delete", "template delete is not configured")
	}

	tmpl, err := s.templates.FindByID(ctx, teamID, templateID)
	if err != nil {
		return serviceerrors.MapRepository(err)
	}

	if err := s.sync.DeleteOnMeta(ctx, teamID, tmpl); err != nil {
		return err
	}
	return serviceerrors.MapRepository(s.templates.SoftDelete(ctx, teamID, templateID))
}

func (s *SyncService) DeleteOnMeta(ctx context.Context, teamID uuid.UUID, tmpl *Template) error {
	if tmpl == nil {
		return serviceerrors.NewValidation("template_id", "template is required")
	}
	deleter, ok := s.meta.(interface {
		DeleteMessageTemplate(context.Context, string, string, string, string) error
	})
	if !ok || s.meta == nil {
		return serviceerrors.NewValidation("delete", "Meta template delete client is not configured")
	}

	ch, creds, token, err := s.loadChannelCredentials(ctx, teamID, tmpl.ChannelID)
	if err != nil {
		return err
	}
	if ch.ChannelType == channel.TypeInstagram {
		return serviceerrors.NewValidation("channel_id", "message templates are only supported for WhatsApp channels")
	}
	if err := channel.ValidateForMeta(ch, creds, token); err != nil {
		return err
	}

	name := strings.TrimSpace(tmpl.Name)
	hsmID := strings.TrimSpace(tmpl.MetaTemplateID)
	if name == "" && hsmID == "" {
		return serviceerrors.NewValidation("template_id", "template is missing Meta identity")
	}

	wabaID := stringValue(ch.WABAID)
	err = metapkg.Retry(ctx, func(retryCtx context.Context) error {
		return deleter.DeleteMessageTemplate(retryCtx, wabaID, token, name, hsmID)
	})
	if err != nil {
		if metapkg.IsInvalidToken(err) {
			return serviceerrors.NewValidation("channel_id", "Meta access token is invalid or expired")
		}
		if isMetaTemplateAlreadyGone(err) {
			return nil
		}
		return serviceerrors.NewValidation("meta", formatMetaTemplateDeleteError(err))
	}
	return nil
}

func isMetaTemplateAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	if metapkg.IsMissingResource(err) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "could not be found") ||
		(strings.Contains(lower, "invalid parameter") && strings.Contains(lower, "hsm"))
}

func formatMetaTemplateDeleteError(err error) string {
	var apiErr *metapkg.APIError
	if errors.As(err, &apiErr) {
		body := strings.TrimSpace(apiErr.Body)
		if body != "" {
			return body
		}
	}
	return err.Error()
}
