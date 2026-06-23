//go:build legacy

package campaign

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/campaignqueue"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type broadcastRecorder interface {
	RecordBroadcastTemplate(ctx context.Context, input message.RecordBroadcastTemplateInput) error
}

type Worker struct {
	campaigns Repository
	contacts  contact.Repository
	devices   device.Repository
	templates template.Repository
	queue     campaignqueue.Queue
	meta      *metaclient.Client
	messages  broadcastRecorder
	log       *logger.Logger
}

func NewWorker(
	campaigns Repository,
	contacts contact.Repository,
	devices device.Repository,
	templates template.Repository,
	queue campaignqueue.Queue,
	meta *metaclient.Client,
	messages broadcastRecorder,
	log *logger.Logger,
) *Worker {
	return &Worker{
		campaigns: campaigns,
		contacts:  contacts,
		devices:   devices,
		templates: templates,
		queue:     queue,
		meta:      meta,
		messages:  messages,
		log:       log,
	}
}

func (w *Worker) Start(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		campaignID, err := w.queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.log.Error("campaign dequeue failed", "error", err)
			continue
		}
		if campaignID == uuid.Nil {
			continue
		}
		if err := w.processCampaign(ctx, campaignID); err != nil {
			w.log.Error("campaign processing failed", "campaign_id", campaignID, "error", err)
		}
	}
}

func (w *Worker) processCampaign(ctx context.Context, campaignID uuid.UUID) error {
	campaign, err := w.campaigns.FindByIDOnly(ctx, campaignID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	campaign.Status = StatusSending
	campaign.StartedAt = &now
	campaign.UpdatedAt = now
	if err := w.campaigns.Update(ctx, campaign); err != nil {
		return err
	}

	dev, err := w.devices.FindByID(ctx, campaign.TeamID, campaign.DeviceID)
	if err != nil {
		return w.failCampaign(ctx, campaign, err)
	}
	if dev.PhoneNumberID == nil || dev.AccessToken == nil || dev.Status != device.StatusConnected {
		return w.failCampaign(ctx, campaign, ErrDeviceNotConnected)
	}

	tpl, err := w.templates.FindByID(ctx, campaign.TeamID, campaign.TemplateID)
	if err != nil {
		return w.failCampaign(ctx, campaign, err)
	}

	mapping, err := parseVariableMapping(campaign.VariableMapping)
	if err != nil {
		return w.failCampaign(ctx, campaign, err)
	}

	pending, err := w.campaigns.ListPendingContacts(ctx, campaign.ID)
	if err != nil {
		return err
	}

	for i := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cc := &pending[i]
		if err := w.sendToContact(ctx, campaign, dev, tpl, mapping, cc); err != nil {
			w.log.Error("campaign send contact failed", "campaign_id", campaign.ID, "contact_id", cc.ID, "error", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	done := time.Now().UTC()
	campaign.Status = StatusCompleted
	campaign.CompletedAt = &done
	campaign.UpdatedAt = done
	if err := w.campaigns.RecalculateCounts(ctx, campaign.ID); err != nil {
		return err
	}
	updated, err := w.campaigns.FindByIDOnly(ctx, campaign.ID)
	if err != nil {
		return err
	}
	campaign.SentCount = updated.SentCount
	campaign.FailedCount = updated.FailedCount
	campaign.DeliveredCount = updated.DeliveredCount
	campaign.ReadCount = updated.ReadCount
	return w.campaigns.Update(ctx, campaign)
}

func (w *Worker) sendToContact(
	ctx context.Context,
	campaign *Campaign,
	dev *device.Device,
	tpl *template.Template,
	mapping VariableMapping,
	cc *CampaignContact,
) error {
	var c *contact.Contact
	if cc.ContactID != nil {
		found, err := w.contacts.FindByID(ctx, campaign.TeamID, *cc.ContactID)
		if err != nil && !errors.Is(err, contact.ErrNotFound) {
			return err
		}
		c = found
	}
	if c == nil {
		c = &contact.Contact{Name: "", Phone: cc.Phone}
	}

	components := buildTemplateComponents(mapping, c)
	wamid, err := w.meta.SendTemplateMessage(
		ctx,
		*dev.PhoneNumberID,
		*dev.AccessToken,
		cc.Phone,
		tpl.Name,
		tpl.Language,
		components,
	)

	now := time.Now().UTC()
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		cc.Status = ContactFailed
		cc.ErrorMessage = &msg
		if updateErr := w.campaigns.UpdateContact(ctx, cc); updateErr != nil {
			return updateErr
		}
		return w.campaigns.IncrementCounts(ctx, campaign.ID, 0, 1)
	}

	cc.Status = ContactSent
	cc.Wamid = &wamid
	cc.SentAt = &now
	if err := w.campaigns.UpdateContact(ctx, cc); err != nil {
		return err
	}
	if err := w.campaigns.IncrementCounts(ctx, campaign.ID, 1, 0); err != nil {
		return err
	}

	if w.messages != nil {
		content := renderTemplateContent(tpl, mapping, c)
		if err := w.messages.RecordBroadcastTemplate(ctx, message.RecordBroadcastTemplateInput{
			TeamID:      campaign.TeamID,
			Device:      dev,
			ContactID:   cc.ContactID,
			Phone:       cc.Phone,
			ContactName: c.Name,
			WAMID:       wamid,
			Content:     content,
			SentAt:      now,
		}); err != nil {
			w.log.Error(
				"campaign conversation history failed",
				"campaign_id", campaign.ID,
				"contact_id", cc.ID,
				"error", err,
			)
		}
	}
	return nil
}

func (w *Worker) failCampaign(ctx context.Context, campaign *Campaign, cause error) error {
	now := time.Now().UTC()
	campaign.Status = StatusFailed
	campaign.CompletedAt = &now
	campaign.UpdatedAt = now
	_ = cause
	return w.campaigns.Update(ctx, campaign)
}
