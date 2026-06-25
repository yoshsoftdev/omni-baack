package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

type recoverySendRepo struct {
	mockSendRepo
	recovered int
}

func (m *recoverySendRepo) RecoverStaleSendingRecipients(_ context.Context, _ time.Time, _ int) (int, error) {
	m.recovered = 1
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.recipients {
		if m.recipients[i].Status == campaign.RecipientSending && m.recipients[i].WAMID == nil {
			m.recipients[i].Status = campaign.RecipientQueued
		}
	}
	return 1, nil
}

func TestRecoverStaleSendingRecipientRequeued(t *testing.T) {
	send := &recoverySendRepo{
		mockSendRepo: mockSendRepo{
			recipients: []campaign.Recipient{{
				ID: uuid.New(), Status: campaign.RecipientSending, ContactID: uuid.New(), Phone: "1",
			}},
		},
	}
	metrics := NewMetrics()
	recovery := NewRecipientRecovery(send, metrics, 15*time.Minute, nil)
	if err := recovery.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if send.recipients[0].Status != campaign.RecipientQueued {
		t.Fatalf("expected queued, got %s", send.recipients[0].Status)
	}
	if metrics.RecipientRecovery != 1 {
		t.Fatalf("expected recovery metric")
	}
}

func TestRecoverSkipsSentRecipient(t *testing.T) {
	wamid := "wamid.done"
	send := &recoverySendRepo{
		mockSendRepo: mockSendRepo{
			recipients: []campaign.Recipient{{
				ID: uuid.New(), Status: campaign.RecipientSent, WAMID: &wamid, ContactID: uuid.New(), Phone: "1",
			}},
		},
	}
	recovery := NewRecipientRecovery(send, NewMetrics(), 15*time.Minute, nil)
	if err := recovery.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if send.recipients[0].Status != campaign.RecipientSent {
		t.Fatalf("sent recipient must remain untouched")
	}
}
