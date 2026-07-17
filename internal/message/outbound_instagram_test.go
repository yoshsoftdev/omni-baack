package message

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
)

type instagramContactStoreStub struct {
	contact *contact.Contact
	scoped  string
}

func (s *instagramContactStoreStub) FindByID(context.Context, uuid.UUID, uuid.UUID) (*contact.Contact, error) {
	return s.contact, nil
}

func (s *instagramContactStoreStub) ResolveInstagramScopedID(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return s.scoped, nil
}

func TestDeliverInstagram_ResolvesScopedIDFromProviderProfile(t *testing.T) {
	teamID := uuid.New()
	contactID := uuid.New()
	contactEntity := &contact.Contact{
		ID:     contactID,
		TeamID: teamID,
		Phone:  "",
	}
	store := &instagramContactStoreStub{
		contact: contactEntity,
		scoped:  "1028733076273916",
	}

	svc := &Service{
		outbound: &OutboundDeps{
			Contacts: store,
		},
	}

	if _, ok := contact.ParseInstagramScopedID(contactEntity.Phone); ok {
		t.Fatal("expected missing ig phone prefix")
	}
	resolvedID, err := svc.outbound.Contacts.ResolveInstagramScopedID(context.Background(), teamID, contactID)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedID != "1028733076273916" {
		t.Fatalf("scoped id = %q", resolvedID)
	}
}
