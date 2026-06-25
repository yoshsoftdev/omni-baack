package channel_test

import (
	"testing"
	"time"

	"github.com/omnichannel/omni-chan-v2/internal/channel"
)

func TestValidateForMeta(t *testing.T) {
	waba := "waba-1"
	phone := "phone-1"
	ch := &channel.Channel{
		Status:        channel.StatusConnected,
		WABAID:        &waba,
		PhoneNumberID: &phone,
	}
	creds := &channel.Credentials{AccessTokenEnc: []byte("enc")}
	if err := channel.ValidateForMeta(ch, creds, "token"); err != nil {
		t.Fatalf("expected valid channel: %v", err)
	}

	expired := time.Now().UTC().Add(-time.Hour)
	credsExpired := &channel.Credentials{AccessTokenEnc: []byte("enc"), TokenExpiresAt: &expired}
	if err := channel.ValidateForMeta(ch, credsExpired, "token"); err == nil {
		t.Fatal("expected expired token error")
	}
}
