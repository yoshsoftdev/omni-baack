package message

import (
	"strings"
	"testing"

	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

func TestValidateAttachment(t *testing.T) {
	contentType, err := ValidateAttachment("image/jpeg", 1024)
	if err != nil || contentType != ContentImage {
		t.Fatalf("ValidateAttachment() = %v, %v", contentType, err)
	}

	_, err = ValidateAttachment("image/gif", 1024)
	if err == nil {
		t.Fatal("expected unsupported mime error")
	}

	_, err = ValidateAttachment("image/jpeg", maxImageBytes+1)
	if err == nil {
		t.Fatal("expected size error")
	}

	_, err = ValidateAttachment("image/jpeg", 0)
	if err == nil {
		t.Fatal("expected empty upload error")
	}
}

func TestValidateAttachmentForChannel_InstagramVideo(t *testing.T) {
	ct, err := ValidateAttachmentForChannel("video/mp4", 20*1024*1024, conversation.ChannelInstagram)
	if err != nil || ct != ContentVideo {
		t.Fatalf("mp4 = %v, %v", ct, err)
	}

	ct, err = ValidateAttachmentForChannel("video/quicktime", 1024, conversation.ChannelInstagram)
	if err != nil || ct != ContentVideo {
		t.Fatalf("mov = %v, %v", ct, err)
	}

	_, err = ValidateAttachmentForChannel("video/3gpp", 1024, conversation.ChannelInstagram)
	if err == nil || !strings.Contains(err.Error(), "3GPP") {
		t.Fatalf("expected 3gpp rejection, got %v", err)
	}

	_, err = ValidateAttachmentForChannel("video/mp4", 20*1024*1024, conversation.ChannelWhatsApp)
	if err == nil {
		t.Fatal("expected whatsapp 16MB video limit")
	}
}
