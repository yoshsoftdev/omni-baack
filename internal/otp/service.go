package otp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/phone"
)

var (
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDeviceNotConnected = errors.New("device is not connected")
	ErrPhoneRequired      = errors.New("phone is required")
	ErrPhoneInvalid       = phone.ErrInvalidIndonesiaFormat
	ErrCodeRequired       = errors.New("code is required")
	ErrCodeInvalid        = errors.New("code must contain digits only")
)

type TemplateMessenger interface {
	SendTemplateMessage(
		ctx context.Context,
		phoneNumberID, accessToken, to, templateName, language string,
		components []metaclient.TemplateComponent,
	) (string, error)
}

type DeviceLookup interface {
	FindBySlug(ctx context.Context, slug string) (*device.Device, error)
}

type Service struct {
	logs              Repository
	devices           DeviceLookup
	meta              TemplateMessenger
	defaultDeviceSlug string
}

func NewService(
	logs Repository,
	devices DeviceLookup,
	meta TemplateMessenger,
	defaultDeviceSlug string,
) *Service {
	slug := strings.TrimSpace(defaultDeviceSlug)
	if slug == "" {
		slug = "dgos-waba"
	}
	return &Service{
		logs:              logs,
		devices:           devices,
		meta:              meta,
		defaultDeviceSlug: slug,
	}
}

type SendInput struct {
	DeviceID string
	Phone    string
	Code     string
}

func (s *Service) Send(ctx context.Context, input SendInput) (*SendResponse, error) {
	deviceSlug := strings.TrimSpace(input.DeviceID)
	if deviceSlug == "" {
		deviceSlug = s.defaultDeviceSlug
	}

	phoneE164, err := phone.ValidateIndonesiaE164(input.Phone)
	if err != nil {
		if strings.TrimSpace(input.Phone) == "" {
			return nil, ErrPhoneRequired
		}
		return nil, ErrPhoneInvalid
	}

	code := strings.TrimSpace(input.Code)
	if code == "" {
		return nil, ErrCodeRequired
	}
	if err := phone.ValidateNumericCode(code); err != nil {
		return nil, ErrCodeInvalid
	}

	dev, err := s.devices.FindBySlug(ctx, deviceSlug)
	if err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	if dev.Status != device.StatusConnected || dev.AccessToken == nil || dev.PhoneNumberID == nil {
		return nil, ErrDeviceNotConnected
	}

	now := time.Now().UTC()
	logEntry := &Log{
		ID:        uuid.New(),
		TeamID:    dev.TeamID,
		DeviceID:  dev.ID,
		Phone:     phoneE164,
		Status:    StatusSent,
		CreatedAt: now,
		UpdatedAt: now,
	}

	components := metaclient.BuildKodeOTPComponents(code)
	wamid, sendErr := s.meta.SendTemplateMessage(
		ctx,
		*dev.PhoneNumberID,
		*dev.AccessToken,
		phone.ToMetaRecipient(phoneE164),
		metaclient.OTPTemplateName,
		metaclient.OTPTemplateLanguage,
		components,
	)

	if sendErr != nil {
		msg := strings.TrimSpace(sendErr.Error())
		logEntry.Status = StatusFailed
		logEntry.ErrorMessage = &msg
		_ = s.logs.Create(ctx, logEntry)
		return nil, sendErr
	}

	logEntry.Wamid = &wamid
	if err := s.logs.Create(ctx, logEntry); err != nil {
		return nil, err
	}

	return &SendResponse{
		ID:     logEntry.ID,
		Phone:  phoneE164,
		Wamid:  wamid,
		Status: StatusSent,
	}, nil
}
