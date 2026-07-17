package provider

import "errors"

var (
	ErrSourceOfTruthMustBeWhatsApp = errors.New("source of truth must be a WhatsApp provider profile")
	ErrCannotLinkToSameContact     = errors.New("provider profile is already linked to this contact")
)
