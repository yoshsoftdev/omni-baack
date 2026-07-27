package template_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type pageMetaLister struct {
	pages map[string][]metapkg.MessageTemplate
	order []string
}

func (m *pageMetaLister) ListMessageTemplatesPage(_ context.Context, _, _, after string) ([]metapkg.MessageTemplate, string, error) {
	if after == "" {
		if len(m.order) == 0 {
			return nil, "", nil
		}
		after = m.order[0]
	}
	items := m.pages[after]
	idx := -1
	for i, key := range m.order {
		if key == after {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, "", nil
	}
	next := ""
	if idx+1 < len(m.order) {
		next = m.order[idx+1]
	}
	return items, next, nil
}

type stubChannelRepo struct {
	ch    *channel.Channel
	creds *channel.Credentials
}

func (s stubChannelRepo) Create(context.Context, *channel.Channel) error { return nil }
func (s stubChannelRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*channel.Channel, error) {
	return s.ch, nil
}
func (s stubChannelRepo) List(context.Context, uuid.UUID, channel.ListFilter, repository.PageParams) (repository.Page[channel.Channel], error) {
	return repository.Page[channel.Channel]{}, nil
}
func (s stubChannelRepo) Update(context.Context, *channel.Channel) error                { return nil }
func (s stubChannelRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error        { return nil }
func (s stubChannelRepo) CountByTeam(context.Context, uuid.UUID) (int64, error)         { return 0, nil }
func (s stubChannelRepo) UpsertCredentials(context.Context, *channel.Credentials) error { return nil }
func (s stubChannelRepo) FindCredentials(context.Context, uuid.UUID) (*channel.Credentials, error) {
	return s.creds, nil
}
func (s stubChannelRepo) DeleteCredentials(context.Context, uuid.UUID) error          { return nil }
func (s stubChannelRepo) UpsertOnboarding(context.Context, *channel.Onboarding) error { return nil }
func (s stubChannelRepo) FindOnboarding(context.Context, uuid.UUID) (*channel.Onboarding, error) {
	return &channel.Onboarding{Status: channel.OnboardingCompleted}, nil
}
func (s stubChannelRepo) CountMetaReady(context.Context, uuid.UUID) (int64, error) { return 1, nil }
func (s stubChannelRepo) FindByPhoneNumberID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindByWABAID(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindByIGUserID(context.Context, uuid.UUID, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindByIGUserIDGlobal(context.Context, string) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) FindConnectedByType(context.Context, uuid.UUID, channel.ChannelType) (*channel.Channel, error) {
	return nil, repository.ErrNotFound
}
func (s stubChannelRepo) UpdateHealthMetadata(context.Context, uuid.UUID, []byte) error { return nil }

type memoryTemplateRepo struct {
	byMeta  map[string]*template.Template
	upserts int
}

func newMemoryTemplateRepo() *memoryTemplateRepo {
	return &memoryTemplateRepo{byMeta: make(map[string]*template.Template)}
}

func (m *memoryTemplateRepo) key(channelID uuid.UUID, metaID string) string {
	return channelID.String() + ":" + metaID
}

func (m *memoryTemplateRepo) Create(context.Context, *template.Template) error { return nil }
func (m *memoryTemplateRepo) Save(ctx context.Context, t *template.Template) error {
	return m.Create(ctx, t)
}
func (m *memoryTemplateRepo) Update(context.Context, *template.Template) error { return nil }
func (m *memoryTemplateRepo) Delete(ctx context.Context, teamID, id uuid.UUID) error {
	return m.SoftDelete(ctx, teamID, id)
}
func (m *memoryTemplateRepo) FindByID(context.Context, uuid.UUID, uuid.UUID) (*template.Template, error) {
	return nil, repository.ErrNotFound
}
func (m *memoryTemplateRepo) FindByIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]*template.Template, error) {
	return map[uuid.UUID]*template.Template{}, nil
}
func (m *memoryTemplateRepo) FindByName(_ context.Context, teamID, channelID uuid.UUID, name, language string) (*template.Template, error) {
	for _, tmpl := range m.byMeta {
		if tmpl.TeamID == teamID && tmpl.ChannelID == channelID && tmpl.Name == name && tmpl.Language == language {
			cp := *tmpl
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (m *memoryTemplateRepo) FindByMetaID(ctx context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*template.Template, error) {
	return m.FindByMetaTemplateID(ctx, teamID, channelID, metaTemplateID)
}
func (m *memoryTemplateRepo) FindByMetaTemplateID(_ context.Context, teamID, channelID uuid.UUID, metaTemplateID string) (*template.Template, error) {
	tmpl, ok := m.byMeta[m.key(channelID, metaTemplateID)]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *tmpl
	return &cp, nil
}
func (m *memoryTemplateRepo) List(context.Context, uuid.UUID, template.ListFilter, repository.PageParams) (repository.Page[template.Template], error) {
	return repository.Page[template.Template]{}, nil
}
func (m *memoryTemplateRepo) UpsertFromSync(_ context.Context, t *template.Template) error {
	m.upserts++
	cp := *t
	m.byMeta[m.key(t.ChannelID, t.MetaTemplateID)] = &cp
	return nil
}
func (m *memoryTemplateRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memoryTemplateRepo) SoftDeleteExceptMetaIDs(_ context.Context, _, channelID uuid.UUID, keepMetaIDs []string) (int64, error) {
	keep := make(map[string]struct{}, len(keepMetaIDs))
	for _, id := range keepMetaIDs {
		keep[id] = struct{}{}
	}
	var removed int64
	for key, tmpl := range m.byMeta {
		if tmpl.ChannelID != channelID {
			continue
		}
		if _, ok := keep[tmpl.MetaTemplateID]; !ok {
			delete(m.byMeta, key)
			removed++
		}
	}
	return removed, nil
}
func (m *memoryTemplateRepo) HasRecentSync(context.Context, uuid.UUID, time.Duration) (bool, error) {
	return m.upserts > 0, nil
}
func (m *memoryTemplateRepo) UpdateFromWebhook(_ context.Context, teamID, channelID uuid.UUID, metaTemplateID, name string, status template.Status, category *string, updatedAt time.Time) error {
	key := m.key(channelID, metaTemplateID)
	tmpl, ok := m.byMeta[key]
	if !ok {
		return repository.ErrNotFound
	}
	if name != "" {
		tmpl.Name = name
	}
	tmpl.Status = status
	if category != nil {
		tmpl.Category = category
	}
	tmpl.UpdatedAt = updatedAt
	return nil
}

func newTestSyncService(meta *pageMetaLister, tmplRepo *memoryTemplateRepo) *template.SyncService {
	waba := "waba-1"
	phone := "phone-1"
	teamID := uuid.New()
	channelID := uuid.New()
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	enc, _ := cipher.Encrypt("token")
	chRepo := stubChannelRepo{
		ch:    &channel.Channel{ID: channelID, TeamID: teamID, Status: channel.StatusConnected, WABAID: &waba, PhoneNumberID: &phone},
		creds: &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
	}
	return template.NewSyncService(meta, chRepo, tmplRepo, template.NewInMemorySyncRunRepository(), cipher, nil)
}

func TestMemoryRepositorySatisfiesDomainAPI(t *testing.T) {
	var _ template.Repository = (*memoryTemplateRepo)(nil)

	repo := newMemoryTemplateRepo()
	teamID := uuid.New()
	channelID := uuid.New()
	tmpl := &template.Template{
		ID:             uuid.New(),
		TeamID:         teamID,
		ChannelID:      channelID,
		MetaTemplateID: "meta-1",
		Name:           "welcome",
		Language:       "en_US",
		Status:         template.StatusApproved,
	}
	ctx := context.Background()
	if err := repo.UpsertFromSync(ctx, tmpl); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByMetaID(ctx, teamID, channelID, "meta-1")
	if err != nil || got.Name != "welcome" {
		t.Fatalf("FindByMetaID: %#v err=%v", got, err)
	}
	byName, err := repo.FindByName(ctx, teamID, channelID, "welcome", "en_US")
	if err != nil || byName.MetaTemplateID != "meta-1" {
		t.Fatalf("FindByName: %#v err=%v", byName, err)
	}
}

func TestSyncServicePagination(t *testing.T) {
	meta := &pageMetaLister{
		order: []string{"page1", "page2", "page3"},
		pages: map[string][]metapkg.MessageTemplate{
			"page1": {{ID: "m1", Name: "a", Language: "en_US", Status: "APPROVED", Components: []byte(`[]`)}},
			"page2": {{ID: "m2", Name: "b", Language: "en_US", Status: "APPROVED", Components: []byte(`[]`)}},
			"page3": {{ID: "m3", Name: "c", Language: "en_US", Status: "APPROVED", Components: []byte(`[]`)}},
		},
	}
	tmplRepo := newMemoryTemplateRepo()
	svc := newTestSyncService(meta, tmplRepo)
	teamID := uuid.New()
	channelID := uuid.New()
	waba := "waba-1"
	phone := "phone-1"
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	enc, _ := cipher.Encrypt("token")
	chRepo := stubChannelRepo{
		ch:    &channel.Channel{ID: channelID, TeamID: teamID, Status: channel.StatusConnected, WABAID: &waba, PhoneNumberID: &phone},
		creds: &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
	}
	svc = template.NewSyncService(meta, chRepo, tmplRepo, template.NewInMemorySyncRunRepository(), cipher, nil)

	result, err := svc.SyncChannel(context.Background(), teamID, channelID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Synced != 3 || result.Created != 3 {
		t.Fatalf("synced=%d created=%d", result.Synced, result.Created)
	}
}

func TestSyncServiceIdempotent(t *testing.T) {
	meta := &pageMetaLister{
		order: []string{"page1"},
		pages: map[string][]metapkg.MessageTemplate{
			"page1": {{ID: "meta-1", Name: "welcome", Language: "en_US", Status: "APPROVED", Components: []byte(`[]`)}},
		},
	}
	tmplRepo := newMemoryTemplateRepo()
	teamID := uuid.New()
	channelID := uuid.New()
	waba := "waba-1"
	phone := "phone-1"
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	enc, _ := cipher.Encrypt("token")
	chRepo := stubChannelRepo{
		ch:    &channel.Channel{ID: channelID, TeamID: teamID, Status: channel.StatusConnected, WABAID: &waba, PhoneNumberID: &phone},
		creds: &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
	}
	svc := template.NewSyncService(meta, chRepo, tmplRepo, template.NewInMemorySyncRunRepository(), cipher, nil)

	for i := 0; i < 3; i++ {
		result, err := svc.SyncChannel(context.Background(), teamID, channelID)
		if err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
		if i > 0 && (result.Created != 0 || result.Updated != 0) {
			t.Fatalf("run %d created=%d updated=%d", i, result.Created, result.Updated)
		}
	}
	if len(tmplRepo.byMeta) != 1 {
		t.Fatalf("templates in db = %d", len(tmplRepo.byMeta))
	}
}

func TestSyncServiceDriftRename(t *testing.T) {
	meta := &pageMetaLister{
		order: []string{"page1"},
		pages: map[string][]metapkg.MessageTemplate{
			"page1": {{ID: "meta-1", Name: "welcome_v2", Language: "en_US", Status: "APPROVED", Components: []byte(`[]`)}},
		},
	}
	tmplRepo := newMemoryTemplateRepo()
	teamID := uuid.New()
	channelID := uuid.New()
	waba := "waba-1"
	phone := "phone-1"
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	enc, _ := cipher.Encrypt("token")
	chRepo := stubChannelRepo{
		ch:    &channel.Channel{ID: channelID, TeamID: teamID, Status: channel.StatusConnected, WABAID: &waba, PhoneNumberID: &phone},
		creds: &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
	}
	svc := template.NewSyncService(meta, chRepo, tmplRepo, template.NewInMemorySyncRunRepository(), cipher, nil)

	now := time.Now().UTC()
	tmplRepo.byMeta[tmplRepo.key(channelID, "meta-1")] = &template.Template{
		ID:             uuid.New(),
		ChannelID:      channelID,
		TeamID:         teamID,
		MetaTemplateID: "meta-1",
		Name:           "welcome",
		Language:       "en_US",
		Status:         template.StatusApproved,
		Components:     json.RawMessage(`[]`),
		LastSyncAt:     &now,
	}

	result, err := svc.SyncChannel(context.Background(), teamID, channelID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated=%d", result.Updated)
	}
	stored := tmplRepo.byMeta[tmplRepo.key(channelID, "meta-1")]
	if stored.Name != "welcome_v2" {
		t.Fatalf("name=%s", stored.Name)
	}
}

func TestSyncChannelFailureDoesNotPanic(t *testing.T) {
	meta := &pageMetaLister{order: []string{"page1"}, pages: map[string][]metapkg.MessageTemplate{}}
	tmplRepo := newMemoryTemplateRepo()
	teamID := uuid.New()
	channelID := uuid.New()
	svc := newTestSyncService(meta, tmplRepo)

	// stubChannelRepo returns connected channel but executeSync fails on missing credentials path
	// by using channel without credentials in a custom repo
	chRepo := stubChannelRepo{
		ch: &channel.Channel{
			ID: channelID, TeamID: teamID, Status: channel.StatusConnected,
			WABAID: ptr("waba"), PhoneNumberID: ptr("phone"),
		},
		creds: nil,
	}
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	svc = template.NewSyncService(meta, chRepo, tmplRepo, template.NewInMemorySyncRunRepository(), cipher, nil)

	_, err := svc.SyncChannel(context.Background(), teamID, channelID)
	if err == nil {
		t.Fatal("expected error")
	}
}

func ptr(s string) *string { return &s }

func TestSyncServiceDriftDelete(t *testing.T) {
	meta := &pageMetaLister{order: []string{"page1"}, pages: map[string][]metapkg.MessageTemplate{"page1": {}}}
	tmplRepo := newMemoryTemplateRepo()
	teamID := uuid.New()
	channelID := uuid.New()
	waba := "waba-1"
	phone := "phone-1"
	cipher, _ := cryptoutil.NewTokenCipherFromBase64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	enc, _ := cipher.Encrypt("token")
	chRepo := stubChannelRepo{
		ch:    &channel.Channel{ID: channelID, TeamID: teamID, Status: channel.StatusConnected, WABAID: &waba, PhoneNumberID: &phone},
		creds: &channel.Credentials{ChannelID: channelID, AccessTokenEnc: enc},
	}
	svc := template.NewSyncService(meta, chRepo, tmplRepo, template.NewInMemorySyncRunRepository(), cipher, nil)
	tmplRepo.byMeta[tmplRepo.key(channelID, "gone")] = &template.Template{
		ID: uuid.New(), ChannelID: channelID, TeamID: teamID, MetaTemplateID: "gone", Name: "old",
	}

	result, err := svc.SyncChannel(context.Background(), teamID, channelID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Removed != 1 || len(tmplRepo.byMeta) != 0 {
		t.Fatalf("removed=%d remaining=%d", result.Removed, len(tmplRepo.byMeta))
	}
}
