package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
)

// TemplateResolver loads domain Templates and builds Meta send components.
// Inbox / OTP / Automation should use this — never pkg/meta.BuildTemplatePayload.
type TemplateResolver struct {
	templates template.Repository
}

// New creates a TemplateResolver backed by TemplateRepository.
func New(templates template.Repository) *TemplateResolver {
	return &TemplateResolver{templates: templates}
}

// GetByID loads a template by workspace + id.
func (r *TemplateResolver) GetByID(ctx context.Context, teamID, templateID uuid.UUID) (*template.Template, error) {
	if r == nil || r.templates == nil {
		return nil, fmt.Errorf("template repository is not configured")
	}
	return r.templates.FindByID(ctx, teamID, templateID)
}

// GetByName loads a template by channel + name + language (language normalized via LanguageResolver).
func (r *TemplateResolver) GetByName(
	ctx context.Context,
	teamID, channelID uuid.UUID,
	name, language string,
) (*template.Template, error) {
	if r == nil || r.templates == nil {
		return nil, fmt.Errorf("template repository is not configured")
	}
	storedLang := template.DefaultLanguageResolver().ForStorageFromCreate(language)
	return r.templates.FindByName(ctx, teamID, channelID, strings.TrimSpace(name), storedLang)
}

// Build renders send components for a domain Template + Variables.
func (r *TemplateResolver) Build(tmpl *template.Template, vars builder.Variables) ([]builder.SendComponent, error) {
	return builder.Build(tmpl, vars)
}

// MetaSendLanguage returns language.code for Meta send for a stored template language.
func (r *TemplateResolver) MetaSendLanguage(stored string) string {
	return template.ToMetaSendLanguage(stored)
}
