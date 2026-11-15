//go:build integration

package contact_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/quota"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
)

type noChannels struct{}

func (noChannels) CountByTeam(context.Context, uuid.UUID) (int64, error) { return 0, nil }

// 7.3: import creates new phones, updates existing ones, and reports bad rows by file line.
func TestImportCreatesUpdatesAndReportsRows(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{URL: url, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)

	teamID, userID := uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO teams (id, name, slug) VALUES ($1, 'it team', $2)`, []any{teamID, "it-" + teamID.String()}},
		{`INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, 'it', 'x')`, []any{userID, userID.String() + "@it.test"}},
		{`INSERT INTO team_members (team_id, user_id, role, role_id) SELECT $1, $2, 'owner', id FROM roles WHERE name = 'owner' LIMIT 1`, []any{teamID, userID}},
	} {
		if _, err := db.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool.Exec(bg, `DELETE FROM contacts WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM team_members WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM teams WHERE id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})

	teams := team.NewRepository(db)
	contacts := contact.NewRepository(db)
	rbacSvc := rbac.NewService(rbac.NewRepository(db))
	if err := rbacSvc.WarmCache(ctx); err != nil {
		t.Fatal(err)
	}
	svc := contact.NewService(contacts, teams, quota.NewService(teams, noChannels{}, contacts), rbacSvc)
	role := string(team.RoleOwner)

	if _, err := svc.Create(ctx, teamID, userID, role, contact.CreateInput{Name: "Lama", Phone: "+6281100000001"}); err != nil {
		t.Fatal(err)
	}

	rows, err := contact.ParseCSV([]byte(
		"name,phone,city\n"+
			"Lama Baru,+6281100000001,Bandung\n"+ // existing → updated
			"Baru,+6281100000002,\n"+ // new → created
			"Tanpa HP,,Medan\n"+ // skipped: no phone
			"Salah,08123,\n"+ // skipped: not E.164
			"Ulang,+6281100000002,Solo\n"), // same phone again in the file → updated
		nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Import(ctx, teamID, userID, role, rows)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Updated != 2 || result.Skipped != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Errors) != 2 || result.Errors[0].Line != 4 || result.Errors[1].Line != 5 {
		t.Fatalf("errors = %+v", result.Errors)
	}

	updated, err := contacts.FindByPhone(ctx, teamID, "+6281100000001")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContactName != "Lama Baru" || updated.City == nil || *updated.City != "Bandung" {
		t.Fatalf("existing contact not updated: %+v", updated)
	}
	created, err := contacts.FindByPhone(ctx, teamID, "+6281100000002")
	if err != nil {
		t.Fatal(err)
	}
	if created.Source != contact.SourceImport || created.City == nil || *created.City != "Solo" {
		t.Fatalf("imported contact = %+v", created)
	}

	// Source and city filter on the server, so totals and paging match.
	source := contact.SourceImport
	page, err := contacts.List(ctx, teamID, contact.ListFilter{Source: &source, City: "sol"}, repository.PageParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Pagination.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("filtered list = %+v", page)
	}
}
