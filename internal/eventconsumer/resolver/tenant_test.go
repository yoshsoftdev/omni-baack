package resolver

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/consumererrors"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

// lookupRepo answers every channel lookup with ch/err; other methods are unused.
type lookupRepo struct {
	channel.Repository
	ch  *channel.Channel
	err error
	got string
}

func (r *lookupRepo) find(key string) (*channel.Channel, error) { r.got = key; return r.ch, r.err }
func (r *lookupRepo) FindByPhoneNumberID(_ context.Context, k string) (*channel.Channel, error) {
	return r.find(k)
}
func (r *lookupRepo) FindByWABAID(_ context.Context, k string) (*channel.Channel, error) {
	return r.find(k)
}
func (r *lookupRepo) FindByIGUserIDGlobal(_ context.Context, k string) (*channel.Channel, error) {
	return r.find(k)
}

func resolvers(r *TenantResolver) map[string]func(context.Context, string) (*TenantContext, error) {
	return map[string]func(context.Context, string) (*TenantContext, error){
		"phone": r.ResolveByPhoneNumberID,
		"waba":  r.ResolveByWABAID,
		"ig":    r.ResolveByIGUserID,
	}
}

func TestResolve_FoundReturnsTenantAndTrimsKey(t *testing.T) {
	ch := &channel.Channel{ID: uuid.New(), TeamID: uuid.New()}
	repo := &lookupRepo{ch: ch}
	for name, resolve := range resolvers(NewTenantResolver(repo)) {
		got, err := resolve(context.Background(), "  key-1 ")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.TeamID != ch.TeamID || got.ChannelID != ch.ID || repo.got != "key-1" {
			t.Fatalf("%s: got %+v, lookup key %q", name, got, repo.got)
		}
	}
}

func TestResolve_UnknownChannelIsTerminal(t *testing.T) {
	repo := &lookupRepo{err: repository.ErrNotFound}
	for name, resolve := range resolvers(NewTenantResolver(repo)) {
		for _, key := range []string{"missing", "   "} {
			_, err := resolve(context.Background(), key)
			if te, ok := consumererrors.IsTerminal(err); !ok || te.Category != "channel" {
				t.Fatalf("%s(%q): expected terminal channel error, got %v", name, key, err)
			}
		}
	}
}

// Regression: a database outage must be retried, not dead-lettered.
func TestResolve_TransientErrorIsRetryable(t *testing.T) {
	dbDown := errors.New("failed to connect to database")
	repo := &lookupRepo{err: dbDown}
	for name, resolve := range resolvers(NewTenantResolver(repo)) {
		_, err := resolve(context.Background(), "key-1")
		if _, terminal := consumererrors.IsTerminal(err); terminal {
			t.Fatalf("%s: transient error became terminal (event would be dead-lettered): %v", name, err)
		}
		if !errors.Is(err, dbDown) {
			t.Fatalf("%s: cause lost: %v", name, err)
		}
	}
}
