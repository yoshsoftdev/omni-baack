package message

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	camrecipient "github.com/omnichannel/omni-chan-v2/internal/campaign/recipient"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/media"
	"github.com/omnichannel/omni-chan-v2/internal/outbound"
)

type stubMediaManager struct {
	ensureCalls int
	lastInput   media.EnsureInput
	ref         *media.MediaReference
	err         error
}

func (s *stubMediaManager) EnsureMedia(_ context.Context, in media.EnsureInput) (*media.MediaReference, error) {
	s.ensureCalls++
	s.lastInput = in
	if s.err != nil {
		return nil, s.err
	}
	return s.ref, nil
}

func (s *stubMediaManager) RegisterAsset(context.Context, media.MediaAsset) error { return nil }
func (s *stubMediaManager) FindAsset(context.Context, uuid.UUID, uuid.UUID) (*media.MediaAsset, error) {
	return nil, errors.New("not found")
}

type stubWhatsAppChannel struct {
	lastMediaID string
	lastTo      string
	textCalls   int
	mediaCalls  int
}

func (c *stubWhatsAppChannel) SendText(_ context.Context, _ outbound.Credentials, msg outbound.TextMessage) (string, error) {
	c.textCalls++
	c.lastTo = msg.To
	return "wamid-text", nil
}

func (c *stubWhatsAppChannel) SendMedia(_ context.Context, _ outbound.Credentials, msg outbound.MediaMessage) (string, error) {
	c.mediaCalls++
	c.lastMediaID = msg.ProviderMediaID
	c.lastTo = msg.To
	return "wamid-media", nil
}

func (c *stubWhatsAppChannel) DeleteMessage(context.Context, outbound.Credentials, string) error {
	return nil
}

type stubWAAddresses struct {
	phone string
	err   error
}

func (s *stubWAAddresses) Resolve(context.Context, uuid.UUID, uuid.UUID) (*camrecipient.WhatsAppRecipient, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &camrecipient.WhatsAppRecipient{PhoneNumber: s.phone, WAID: s.phone}, nil
}

func TestResolveWhatsAppAttachmentUsesEnsureMedia(t *testing.T) {
	t.Parallel()
	assetID := uuid.New()
	teamID := uuid.New()
	mgr := &stubMediaManager{
		ref: &media.MediaReference{AssetID: assetID, ProviderMediaID: "99887766"},
	}
	svc := &Service{outbound: &OutboundDeps{Media: mgr}}

	got, err := svc.resolveWhatsAppAttachmentMediaID(
		context.Background(),
		teamID,
		"phone-1",
		"token",
		&Attachment{StorageObjectID: assetID, MimeType: "image/jpeg", Filename: "a.jpg"},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "99887766" {
		t.Fatalf("media id = %s", got)
	}
	if mgr.ensureCalls != 1 {
		t.Fatalf("EnsureMedia calls = %d", mgr.ensureCalls)
	}
	if mgr.lastInput.AssetID != assetID || mgr.lastInput.TeamID != teamID {
		t.Fatalf("ensure input = %+v", mgr.lastInput)
	}
}

func TestResolveWhatsAppAttachmentSkipsEnsureMediaWhenProviderIDPresent(t *testing.T) {
	t.Parallel()
	mgr := &stubMediaManager{
		ref: &media.MediaReference{ProviderMediaID: "should-not-use"},
	}
	svc := &Service{outbound: &OutboundDeps{Media: mgr}}
	got, err := svc.resolveWhatsAppAttachmentMediaID(
		context.Background(),
		uuid.New(),
		"phone-1",
		"token",
		&Attachment{StorageObjectID: uuid.New()},
		"1234567890",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1234567890" {
		t.Fatalf("got %s", got)
	}
	if mgr.ensureCalls != 0 {
		t.Fatal("EnsureMedia must be skipped for existing cloud media id")
	}
}

func TestDeliverWhatsAppAttachmentUsesEnsureMediaNotUpload(t *testing.T) {
	t.Parallel()
	assetID := uuid.New()
	phoneID := "phone-1"
	mgr := &stubMediaManager{
		ref: &media.MediaReference{AssetID: assetID, ProviderMediaID: "555001"},
	}
	wa := &stubWhatsAppChannel{}
	svc := &Service{
		outbound: &OutboundDeps{
			WhatsApp:          wa,
			Media:             mgr,
			WhatsAppAddresses: &stubWAAddresses{phone: "628123"},
		},
	}
	ch := &channel.Channel{PhoneNumberID: &phoneID}

	_, wamid, err := svc.deliverWhatsApp(
		context.Background(),
		uuid.New(),
		ch,
		uuid.New(),
		"token",
		"caption",
		ContentImage,
		&Attachment{StorageObjectID: assetID, Filename: "x.jpg"},
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if wamid == nil || *wamid != "wamid-media" {
		t.Fatalf("wamid = %v", wamid)
	}
	if wa.lastMediaID != "555001" {
		t.Fatalf("sent media id = %s", wa.lastMediaID)
	}
	if wa.lastTo != "628123" {
		t.Fatalf("to = %s (must come from address resolver)", wa.lastTo)
	}
	if mgr.ensureCalls != 1 {
		t.Fatalf("EnsureMedia calls = %d", mgr.ensureCalls)
	}
	if wa.mediaCalls != 1 {
		t.Fatalf("SendMedia calls = %d", wa.mediaCalls)
	}
}

func TestDeliverWhatsAppUsesAddressResolverNotContactPhone(t *testing.T) {
	t.Parallel()
	phoneID := "phone-1"
	wa := &stubWhatsAppChannel{}
	svc := &Service{
		outbound: &OutboundDeps{
			WhatsApp:          wa,
			WhatsAppAddresses: &stubWAAddresses{phone: "628999888"},
		},
	}
	ch := &channel.Channel{PhoneNumberID: &phoneID}
	recipient, _, err := svc.deliverWhatsApp(
		context.Background(),
		uuid.New(),
		ch,
		uuid.New(),
		"token",
		"hello",
		ContentText,
		nil,
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if recipient == nil || *recipient != "628999888" {
		t.Fatalf("recipient = %v", recipient)
	}
	if wa.textCalls != 1 || wa.lastTo != "628999888" {
		t.Fatalf("send to = %s calls = %d", wa.lastTo, wa.textCalls)
	}
}

func TestDeliverWhatsAppRequiresProvider(t *testing.T) {
	t.Parallel()
	phoneID := "phone-1"
	svc := &Service{
		outbound: &OutboundDeps{
			WhatsApp:          &stubWhatsAppChannel{},
			WhatsAppAddresses: &stubWAAddresses{err: camrecipient.ErrNoWhatsAppProvider},
		},
	}
	ch := &channel.Channel{PhoneNumberID: &phoneID}
	_, _, err := svc.deliverWhatsApp(
		context.Background(),
		uuid.New(),
		ch,
		uuid.New(),
		"token",
		"hello",
		ContentText,
		nil,
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected missing provider error")
	}
}

func TestResolveWhatsAppAttachmentRequiresMediaManager(t *testing.T) {
	t.Parallel()
	svc := &Service{outbound: &OutboundDeps{}}
	_, err := svc.resolveWhatsAppAttachmentMediaID(
		context.Background(),
		uuid.New(),
		"phone-1",
		"token",
		&Attachment{StorageObjectID: uuid.New()},
		"",
	)
	if err == nil {
		t.Fatal("expected error when media manager missing")
	}
}
