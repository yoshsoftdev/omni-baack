package provider

// Kind identifies a messaging or identity provider without leaking provider SDK types.
type Kind string

const (
	WhatsApp  Kind = "whatsapp"
	Instagram Kind = "instagram"
	Messenger Kind = "messenger"
	Telegram  Kind = "telegram"
	Email     Kind = "email"
)

// Status describes the lifecycle of an external account.
type Status string

const (
	StatusActive      Status = "active"
	StatusUnavailable Status = "unavailable"
	StatusUnlinked    Status = "unlinked"
)
