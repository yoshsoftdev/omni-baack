package message_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliverWhatsAppSourceDoesNotUseContactPhone(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "message", "outbound.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "contactEntity.Phone") {
		t.Fatal("outbound must not read Contact.Phone for WhatsApp addressing")
	}
	if !strings.Contains(body, "WhatsAppAddresses.Resolve") {
		t.Fatal("outbound must resolve WhatsApp address via WhatsAppAddresses")
	}
	if !strings.Contains(body, "ResolveInstagramScopedID") {
		t.Fatal("instagram outbound must use ResolveInstagramScopedID")
	}
}
