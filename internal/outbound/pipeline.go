package outbound

// Inbox outbound pipeline (Sprint 4 Phase 7):
//
//	WhatsAppRecipientResolver  → address (never Contact.Phone)
//	TemplateResolver + Builder → optional Meta template components (when used)
//	MediaManager.EnsureMedia   → WhatsApp attachment media ids
//	Channel.Send               → WhatsAppChannel / InstagramChannel
//	Meta Graph                 → only inside Channel adapters
//
// Free-form Inbox replies skip Template Builder; attachments still use MediaManager.
// Campaign broadcast uses the same MediaManager + recipient resolver + builder.
const PipelineDoc = "resolver → builder → media → channel → meta"
