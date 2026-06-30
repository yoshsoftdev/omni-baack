package message

import "testing"

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
