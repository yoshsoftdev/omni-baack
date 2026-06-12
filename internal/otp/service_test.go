package otp

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type mockOTPRepo struct {
	created *Log
}

func (m *mockOTPRepo) Create(_ context.Context, log *Log) error {
	m.created = log
	return nil
}

type mockDeviceRepo struct {
	device *device.Device
	slug   string
}

func (m *mockDeviceRepo) FindBySlug(_ context.Context, slug string) (*device.Device, error) {
	if m.device == nil || slug != m.slug {
		return nil, device.ErrNotFound
	}
	return m.device, nil
}

type mockMetaClient struct {
	wamid string
	err   error
	last  struct {
		to           string
		templateName string
		language     string
		components   []metaclient.TemplateComponent
	}
}

func (m *mockMetaClient) SendTemplateMessage(
	_ context.Context,
	_, _, to, templateName, language string,
	components []metaclient.TemplateComponent,
) (string, error) {
	m.last.to = to
	m.last.templateName = templateName
	m.last.language = language
	m.last.components = components
	if m.err != nil {
		return "", m.err
	}
	return m.wamid, nil
}

func TestService_Send_Success(t *testing.T) {
	token := "token"
	phoneNumberID := "pnid"
	deviceID := uuid.New()
	repo := &mockOTPRepo{}
	devRepo := &mockDeviceRepo{
		slug: "dgos-waba",
		device: &device.Device{
			ID:            deviceID,
			TeamID:        uuid.New(),
			Status:        device.StatusConnected,
			AccessToken:   &token,
			PhoneNumberID: &phoneNumberID,
		},
	}
	meta := &mockMetaClient{wamid: "wamid.123"}

	svc := NewService(repo, devRepo, meta, "dgos-waba")

	result, err := svc.Send(context.Background(), SendInput{
		DeviceID: "dgos-waba",
		Phone:    "+628123456789",
		Code:     "464646",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.Wamid != "wamid.123" {
		t.Fatalf("Wamid = %q", result.Wamid)
	}
	if meta.last.to != "628123456789" {
		t.Fatalf("meta to = %q", meta.last.to)
	}
	if meta.last.templateName != metaclient.OTPTemplateName {
		t.Fatalf("template = %q", meta.last.templateName)
	}
	if len(meta.last.components) != 2 {
		t.Fatalf("components = %d", len(meta.last.components))
	}
	if repo.created == nil || repo.created.Status != StatusSent {
		t.Fatal("expected sent log")
	}
}

func TestService_Send_DefaultDeviceSlug(t *testing.T) {
	token := "token"
	phoneNumberID := "pnid"
	devRepo := &mockDeviceRepo{
		slug: "dgos-waba",
		device: &device.Device{
			ID:            uuid.New(),
			TeamID:        uuid.New(),
			Status:        device.StatusConnected,
			AccessToken:   &token,
			PhoneNumberID: &phoneNumberID,
		},
	}
	svc := NewService(&mockOTPRepo{}, devRepo, &mockMetaClient{wamid: "wamid"}, "dgos-waba")

	_, err := svc.Send(context.Background(), SendInput{
		Phone: "+628123456789",
		Code:  "123456",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestService_Send_InvalidPhone(t *testing.T) {
	svc := NewService(&mockOTPRepo{}, &mockDeviceRepo{}, &mockMetaClient{}, "dgos-waba")

	_, err := svc.Send(context.Background(), SendInput{
		DeviceID: "dgos-waba",
		Phone:    "08123456789",
		Code:     "1234",
	})
	if !errors.Is(err, ErrPhoneInvalid) {
		t.Fatalf("expected ErrPhoneInvalid, got %v", err)
	}
}

func TestService_Send_InvalidCode(t *testing.T) {
	token := "token"
	phoneNumberID := "pnid"
	svc := NewService(&mockOTPRepo{}, &mockDeviceRepo{
		slug: "dgos-waba",
		device: &device.Device{
			ID:            uuid.New(),
			Status:        device.StatusConnected,
			AccessToken:   &token,
			PhoneNumberID: &phoneNumberID,
		},
	}, &mockMetaClient{}, "dgos-waba")

	_, err := svc.Send(context.Background(), SendInput{
		DeviceID: "dgos-waba",
		Phone:    "+628123456789",
		Code:     "12ab",
	})
	if !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("expected ErrCodeInvalid, got %v", err)
	}
}

func TestService_Send_DeviceNotFound(t *testing.T) {
	svc := NewService(&mockOTPRepo{}, &mockDeviceRepo{}, &mockMetaClient{}, "dgos-waba")

	_, err := svc.Send(context.Background(), SendInput{
		DeviceID: "unknown-device",
		Phone:    "+628123456789",
		Code:     "1234",
	})
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}
