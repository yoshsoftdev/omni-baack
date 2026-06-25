package channel

import (
	"context"
	"errors"
	"testing"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type stubMetaClient struct {
	pingErr      error
	waba         *metapkg.WABA
	wabaErr      error
	phone        *metapkg.PhoneNumber
	phoneErr     error
	phones       []metapkg.PhoneNumber
	phonesErr    error
	templates    []metapkg.MessageTemplate
	templatesErr error
}

func (s stubMetaClient) Ping(context.Context, string) error { return s.pingErr }
func (s stubMetaClient) GetWABA(context.Context, string, string) (*metapkg.WABA, error) {
	return s.waba, s.wabaErr
}
func (s stubMetaClient) GetPhoneNumber(context.Context, string, string) (*metapkg.PhoneNumber, error) {
	return s.phone, s.phoneErr
}
func (s stubMetaClient) GetPhoneNumbers(context.Context, string, string) ([]metapkg.PhoneNumber, error) {
	return s.phones, s.phonesErr
}
func (s stubMetaClient) ListTemplates(context.Context, string, string) ([]metapkg.MessageTemplate, error) {
	return s.templates, s.templatesErr
}

func TestVerifyManualConnectSuccess(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, stubMetaClient{
		waba:   &metapkg.WABA{ID: "waba-1", Name: "Demo WABA"},
		phone:  &metapkg.PhoneNumber{ID: "phone-1", DisplayPhoneNumber: "+628123"},
		phones: []metapkg.PhoneNumber{{ID: "phone-1"}},
		templates: []metapkg.MessageTemplate{
			{ID: "tpl-1", Name: "hello"},
		},
	})

	result, err := svc.verifyManualConnect(context.Background(), ManualConnectInput{
		AccessToken:   "token",
		WABAID:        "waba-1",
		PhoneNumberID: "phone-1",
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !result.Checks.Token || !result.Checks.WABA || !result.Checks.Phone || !result.Checks.TemplatesAPI {
		t.Fatalf("checks=%+v", result.Checks)
	}
	if result.TemplateCount != 1 || result.WABAName != "Demo WABA" {
		t.Fatalf("result=%+v", result)
	}
}

func TestVerifyManualConnectInvalidToken(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, stubMetaClient{
		pingErr: errors.New("oauth error"),
	})

	_, err := svc.verifyManualConnect(context.Background(), ManualConnectInput{
		AccessToken:   "bad",
		WABAID:        "waba-1",
		PhoneNumberID: "phone-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyManualConnectPhoneNotInWABA(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, stubMetaClient{
		waba:   &metapkg.WABA{ID: "waba-1", Name: "Demo WABA"},
		phone:  &metapkg.PhoneNumber{ID: "phone-2"},
		phones: []metapkg.PhoneNumber{{ID: "phone-1"}},
	})

	_, err := svc.verifyManualConnect(context.Background(), ManualConnectInput{
		AccessToken:   "token",
		WABAID:        "waba-1",
		PhoneNumberID: "phone-2",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
