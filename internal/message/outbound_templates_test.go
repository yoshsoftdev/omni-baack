package message_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
	templateresolver "github.com/omnichannel/omni-chan-v2/internal/template/resolver"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type stubTemplateRepo struct {
	byID map[uuid.UUID]*template.Template
}

func (s *stubTemplateRepo) FindByID(_ context.Context, _, id uuid.UUID) (*template.Template, error) {
	if tmpl, ok := s.byID[id]; ok {
		cp := *tmpl
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}
func (s *stubTemplateRepo) FindByIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]*template.Template, error) {
	return map[uuid.UUID]*template.Template{}, nil
}
func (s *stubTemplateRepo) FindByName(context.Context, uuid.UUID, uuid.UUID, string, string) (*template.Template, error) {
	return nil, repository.ErrNotFound
}
func (s *stubTemplateRepo) FindByMetaID(context.Context, uuid.UUID, uuid.UUID, string) (*template.Template, error) {
	return nil, repository.ErrNotFound
}
func (s *stubTemplateRepo) FindByMetaTemplateID(context.Context, uuid.UUID, uuid.UUID, string) (*template.Template, error) {
	return nil, repository.ErrNotFound
}
func (s *stubTemplateRepo) List(context.Context, uuid.UUID, template.ListFilter, repository.PageParams) (repository.Page[template.Template], error) {
	return repository.Page[template.Template]{}, nil
}
func (s *stubTemplateRepo) Save(context.Context, *template.Template) error   { return nil }
func (s *stubTemplateRepo) Create(context.Context, *template.Template) error { return nil }
func (s *stubTemplateRepo) Update(context.Context, *template.Template) error { return nil }
func (s *stubTemplateRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *stubTemplateRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (s *stubTemplateRepo) UpsertFromSync(context.Context, *template.Template) error {
	return nil
}
func (s *stubTemplateRepo) SoftDeleteExceptMetaIDs(context.Context, uuid.UUID, uuid.UUID, []string) (int64, error) {
	return 0, nil
}
func (s *stubTemplateRepo) HasRecentSync(context.Context, uuid.UUID, time.Duration) (bool, error) {
	return false, nil
}
func (s *stubTemplateRepo) UpdateFromWebhook(context.Context, uuid.UUID, uuid.UUID, string, string, template.Status, *string, time.Time) error {
	return nil
}

func TestOutboundDeps_TemplatesResolverWired(t *testing.T) {
	t.Parallel()
	teamID := uuid.New()
	tmplID := uuid.New()
	tmpl := &template.Template{
		ID:         tmplID,
		TeamID:     teamID,
		Name:       "welcome",
		Language:   "en_US",
		Components: json.RawMessage(`[{"type":"BODY","text":"Hi {{1}}"}]`),
	}
	repo := &stubTemplateRepo{byID: map[uuid.UUID]*template.Template{tmplID: tmpl}}
	resolver := templateresolver.New(repo)

	svc := message.NewService(nil, nil, nil, nil, nil, nil, &message.OutboundDeps{
		Templates: resolver,
	})
	got := svc.Templates()
	if got == nil {
		t.Fatal("Templates() = nil")
	}

	loaded, err := got.GetByID(context.Background(), teamID, tmplID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.Name != "welcome" {
		t.Fatalf("name = %s", loaded.Name)
	}

	components, err := got.Build(loaded, builder.Variables{Body: map[string]string{"1": "Ada"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || components[0].Parameters[0].Text != "Ada" {
		t.Fatalf("components = %#v", components)
	}
	if lang := got.MetaSendLanguage("en_EN"); lang != "en_US" {
		t.Fatalf("MetaSendLanguage = %s", lang)
	}
}

func TestService_TemplatesNilWhenUnwired(t *testing.T) {
	t.Parallel()
	svc := message.NewService(nil, nil, nil, nil, nil, nil, nil)
	if svc.Templates() != nil {
		t.Fatal("expected nil Templates()")
	}
	svc2 := message.NewService(nil, nil, nil, nil, nil, nil, &message.OutboundDeps{})
	if svc2.Templates() != nil {
		t.Fatal("expected nil Templates() when field unset")
	}
}

func TestService_MetaSendLanguageUsesLanguageResolver(t *testing.T) {
	t.Parallel()
	// Unwired: still resolves via LanguageResolver (no hardcoded Meta locales).
	svc := message.NewService(nil, nil, nil, nil, nil, nil, nil)
	if got := svc.MetaSendLanguage("en_EN"); got != "en_US" {
		t.Fatalf("en_EN → %s", got)
	}
	if got := svc.MetaSendLanguage("en"); got != "en_US" {
		t.Fatalf("en → %s", got)
	}
	if got := svc.MetaSendLanguage("id"); got != "id" {
		t.Fatalf("id → %s", got)
	}

	// Wired: same path through TemplateResolver.
	svc2 := message.NewService(nil, nil, nil, nil, nil, nil, &message.OutboundDeps{
		Templates: templateresolver.New(&stubTemplateRepo{}),
	})
	if got := svc2.MetaSendLanguage("en_EN"); got != "en_US" {
		t.Fatalf("wired en_EN → %s", got)
	}
}
