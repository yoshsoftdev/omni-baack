package channelmgmt

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type stubChannelStore struct {
	items      []channel.Channel
	connected  map[channel.ChannelType]*channel.Channel
	listErr    error
	findErr    error
}

func (s *stubChannelStore) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	if s.listErr != nil {
		return repository.Page[channel.Channel]{}, s.listErr
	}
	return repository.Page[channel.Channel]{Items: s.items}, nil
}

func (s *stubChannelStore) FindConnectedByType(_ context.Context, _ uuid.UUID, channelType channel.ChannelType) (*channel.Channel, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	if ch, ok := s.connected[channelType]; ok {
		return ch, nil
	}
	return nil, repository.ErrNotFound
}

func TestListTypes_SupportedWhatsAppAndInstagram(t *testing.T) {
	svc := NewService(nil, nil, nil)
	types, err := svc.ListTypes(context.Background(), uuid.New(), uuid.New(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(types) < 2 {
		t.Fatalf("expected at least whatsapp+instagram, got %d", len(types))
	}
	var wa, ig *ChannelTypeInfo
	for i := range types {
		switch types[i].Type {
		case TypeWhatsApp:
			wa = &types[i]
		case TypeInstagram:
			ig = &types[i]
		}
	}
	if wa == nil || !wa.Supported || wa.DisplayName != "WhatsApp Business" {
		t.Fatalf("unexpected whatsapp type: %+v", wa)
	}
	if ig == nil || !ig.Supported || ig.DisplayName != "Instagram Business" {
		t.Fatalf("unexpected instagram type: %+v", ig)
	}
}

func TestStatus_WhatsAppAndInstagramFromStore(t *testing.T) {
	svc := NewService(&stubChannelStore{
		items: []channel.Channel{
			{Name: "Sales Line 1", Status: channel.StatusConnected, ChannelType: channel.TypeWhatsApp},
		},
		connected: map[channel.ChannelType]*channel.Channel{
			channel.TypeInstagram: {Name: "brand.ig", Status: channel.StatusConnected, ChannelType: channel.TypeInstagram},
		},
	}, nil, nil)

	report, err := svc.Status(context.Background(), uuid.New(), uuid.New(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Channels) != 2 {
		t.Fatalf("expected 2 channel entries, got %d", len(report.Channels))
	}
	if !report.Channels[0].Connected || report.Channels[0].DisplayName != "Sales Line 1" {
		t.Fatalf("unexpected whatsapp status: %+v", report.Channels[0])
	}
	if !report.Channels[1].Connected || report.Channels[1].DisplayName != "brand.ig" {
		t.Fatalf("unexpected instagram status: %+v", report.Channels[1])
	}
}

func TestStatus_WhatsAppDisconnected(t *testing.T) {
	svc := NewService(&stubChannelStore{
		items: []channel.Channel{
			{Name: "Draft", Status: channel.StatusDisconnected, ChannelType: channel.TypeWhatsApp},
		},
		connected: map[channel.ChannelType]*channel.Channel{},
	}, nil, nil)

	report, err := svc.Status(context.Background(), uuid.New(), uuid.New(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if report.Channels[0].Connected {
		t.Fatalf("expected whatsapp disconnected, got %+v", report.Channels[0])
	}
	if report.Channels[1].Connected {
		t.Fatalf("expected instagram disconnected, got %+v", report.Channels[1])
	}
}

func TestInstagramStatus_FromDatabase(t *testing.T) {
	svc := NewService(&stubChannelStore{
		connected: map[channel.ChannelType]*channel.Channel{
			channel.TypeInstagram: {Name: "brand.ig", Status: channel.StatusConnected},
		},
	}, nil, nil)

	status, err := svc.InstagramStatus(context.Background(), uuid.New(), uuid.New(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Connected || status.DisplayName != "brand.ig" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestInstagramStatus_NotConnected(t *testing.T) {
	svc := NewService(&stubChannelStore{connected: map[channel.ChannelType]*channel.Channel{}}, nil, nil)
	status, err := svc.InstagramStatus(context.Background(), uuid.New(), uuid.New(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if status.Connected {
		t.Fatal("expected connected=false")
	}
}
