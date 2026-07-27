package recipient

import (
	"encoding/json"

	"github.com/google/uuid"
)

// SnapshotStatus is the frozen recipient lifecycle status at snapshot time.
type SnapshotStatus string

const (
	SnapshotPending SnapshotStatus = "pending"
	SnapshotSkipped SnapshotStatus = "skipped"
)

// SkipReason explains why a recipient snapshot was marked skipped.
type SkipReason string

const (
	SkipReasonNone               SkipReason = ""
	SkipReasonNoWhatsAppProvider SkipReason = "contact_does_not_have_whatsapp_provider"
	SkipReasonOptOut             SkipReason = "marketing_unsubscribed"
	SkipReasonInvalidAddress     SkipReason = "invalid_whatsapp_address"
)

// Message returns a clear, user-facing skip reason.
func (r SkipReason) Message() string {
	switch r {
	case SkipReasonNoWhatsAppProvider:
		return "Contact does not have a WhatsApp provider."
	case SkipReasonOptOut:
		return "Contact is unsubscribed from marketing."
	case SkipReasonInvalidAddress:
		return "Contact WhatsApp address is invalid."
	default:
		return ""
	}
}

// Snapshot is the frozen recipient row prepared before insert into campaign_recipients.
// Worker must send using this data only — never re-resolve Contact or Provider.
type Snapshot struct {
	ContactID         uuid.UUID
	ProviderProfileID *uuid.UUID
	PhoneNumber       string
	WAID              string
	Variables         map[string]string
	Status            SnapshotStatus
	SkipReason        SkipReason
}

// VariablesJSON encodes Variables for campaign_recipients.variables JSONB.
func (s Snapshot) VariablesJSON() (json.RawMessage, error) {
	if len(s.Variables) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(s.Variables)
}
