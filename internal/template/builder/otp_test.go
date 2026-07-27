package builder_test

import (
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
)

func TestBuildAuthenticationOTP(t *testing.T) {
	t.Parallel()
	components, err := builder.BuildAuthenticationOTP("123456")
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 2 {
		t.Fatalf("len = %d", len(components))
	}
	if components[0].Type != "body" || components[0].Parameters[0].Text != "123456" {
		t.Fatalf("body = %#v", components[0])
	}
	if components[1].Type != "button" || components[1].SubType != "url" {
		t.Fatalf("button = %#v", components[1])
	}
	if _, err := builder.BuildAuthenticationOTP("  "); err == nil {
		t.Fatal("expected empty code error")
	}
	if got := builder.AuthenticationOTPSendLanguage(); got != "id" {
		t.Fatalf("language = %s", got)
	}
	if builder.AuthenticationOTPName != "kode_otp" {
		t.Fatalf("name = %s", builder.AuthenticationOTPName)
	}
}

func TestAuthenticationOTPSendLanguageUsesResolver(t *testing.T) {
	t.Parallel()
	// Must match LanguageResolver — never a raw Meta hardcode that invents en_EN.
	want := template.ToMetaSendLanguage(
		template.DefaultLanguageResolver().ForStorageFromCreate(builder.AuthenticationOTPLanguageStored),
	)
	if got := builder.AuthenticationOTPSendLanguage(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
