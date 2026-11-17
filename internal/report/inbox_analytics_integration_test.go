//go:build integration

package report_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/report"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

// 9.3: a hand-built inbox whose correct numbers are known.
//
//	A: customer writes at T, agent1 replies at T+60s, resolved at T+600s.
//	B: customer writes at T, only an auto-reply (ledger) answers: unanswered, open, assigned to agent2.
//	C: agent2 starts the conversation at T, customer replies at T+100s: not a first-response sample.
func TestInboxAnalyticsInPostgres(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{URL: url, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	teamID, owner, agent1, agent2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contacts := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	convA, convB, convC := uuid.New(), uuid.New(), uuid.New()
	T := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	at := func(s int) time.Time { return T.Add(time.Duration(s) * time.Second) }
	autoMsg := uuid.New()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO teams (id, name, slug) VALUES ($1, 'it', $2)`, teamID, "it-"+teamID.String())
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool.Exec(bg, `DELETE FROM messages WHERE team_id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM teams WHERE id = $1`, teamID)
		_, _ = db.Pool.Exec(bg, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{owner, agent1, agent2})
	})
	for i, u := range []uuid.UUID{owner, agent1, agent2} {
		name := []string{"Owner", "Agent One", "Agent Two"}[i]
		role := []string{"owner", "agent", "agent"}[i]
		exec(`INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, 'x')`, u, u.String()+"@it.test", name)
		exec(`INSERT INTO team_members (team_id, user_id, role, role_id) SELECT $1, $2, $3::text::team_role, id FROM roles WHERE name = $3::text LIMIT 1`, teamID, u, role)
	}
	for _, c := range contacts {
		exec(`INSERT INTO contacts (id, team_id, contact_name, customer_name) VALUES ($1, $2, 'c', 'c')`, c, teamID)
	}
	ptr := func(t time.Time) *time.Time { return &t }
	conv := func(id, contact uuid.UUID, created time.Time, firstResp, resolved *time.Time, status string, assigned uuid.UUID) {
		exec(`INSERT INTO conversations (id, team_id, contact_id, external_thread_id, created_at, first_response_at, resolution_at, status, assigned_to)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::conversation_status, $9)`, id, teamID, contact, id.String(), created, firstResp, resolved, status, assigned)
	}
	conv(convA, contacts[0], at(0), ptr(at(60)), ptr(at(600)), "closed", agent1)
	conv(convB, contacts[1], at(0), nil, nil, "open", agent2)
	conv(convC, contacts[2], at(0), ptr(at(0)), nil, "open", agent2)

	msg := func(id, convID uuid.UUID, dir, sender string, ts time.Time) {
		exec(`INSERT INTO messages (id, conversation_id, team_id, direction, sender, content, received_at, created_at)
			VALUES ($1, $2, $3, $4::message_direction, $5, 'x', $6, $6)`, id, convID, teamID, dir, sender, ts)
	}
	msg(uuid.New(), convA, "inbound", "628111", at(0))
	msg(uuid.New(), convA, "outbound", agent1.String(), at(60))
	msg(uuid.New(), convB, "inbound", "628222", at(0))
	msg(autoMsg, convB, "outbound", owner.String(), at(1)) // welcome auto-reply, sent as the owner
	exec(`INSERT INTO inbox_auto_replies (id, team_id, conversation_id, inbound_message_id, kind, message_id) VALUES ($1, $2, $3, $4, 'welcome', $5)`,
		uuid.New(), teamID, convB, uuid.New(), autoMsg)
	msg(uuid.New(), convC, "outbound", agent2.String(), at(0))
	msg(uuid.New(), convC, "inbound", "628333", at(100))

	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	got, err := report.NewInboxAnalyticsRepository(db).InboxAnalytics(ctx, teamID, report.InboxAnalyticsQuery{
		From: T.Add(-time.Hour), To: T.Add(time.Hour), Location: jakarta,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Summary
	if s.ConversationsStarted != 3 || s.MessagesReceived != 3 || s.MessagesSentByAgents != 2 {
		t.Errorf("volume = started %d, received %d, sent by agents %d; want 3, 3, 2 (auto-reply excluded)",
			s.ConversationsStarted, s.MessagesReceived, s.MessagesSentByAgents)
	}
	if s.FirstResponseMedian == nil || *s.FirstResponseMedian != 60 {
		t.Errorf("first response median = %v, want 60s (agent-started C excluded)", s.FirstResponseMedian)
	}
	if s.Resolved != 1 || s.ResolutionMedian == nil || *s.ResolutionMedian != 600 {
		t.Errorf("resolved = %d, median %v; want 1, 600s", s.Resolved, s.ResolutionMedian)
	}
	if s.Unanswered != 1 || s.OpenNow != 2 {
		t.Errorf("unanswered = %d, open = %d; want 1 (B), 2 (B, C)", s.Unanswered, s.OpenNow)
	}

	byName := map[string]report.AgentStats{}
	for _, a := range got.Agents {
		byName[a.Name] = a
	}
	a1, a2, o := byName["Agent One"], byName["Agent Two"], byName["Owner"]
	if a1.MessagesSent != 1 || a1.Resolved != 1 || a1.FirstResponseMedian == nil || *a1.FirstResponseMedian != 60 {
		t.Errorf("agent one = %+v", a1)
	}
	if a2.MessagesSent != 1 || a2.OpenAssigned != 2 || a2.FirstResponseMedian != nil {
		t.Errorf("agent two = %+v (C is agent-started, so no first-response sample)", a2)
	}
	if o.MessagesSent != 0 {
		t.Errorf("owner messages = %d, want 0: the auto-reply is not their work", o.MessagesSent)
	}
	var daySum int64
	for _, d := range got.Daily {
		daySum += d.MessagesReceived
	}
	if daySum != 3 {
		t.Errorf("daily received sum = %d, want 3 (%+v)", daySum, got.Daily)
	}
}
