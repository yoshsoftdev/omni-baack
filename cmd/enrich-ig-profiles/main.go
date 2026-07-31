package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/cryptoutil"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type row struct {
	ContactID      uuid.UUID
	TeamID         uuid.UUID
	ProfileID      uuid.UUID
	ProviderUserID string
	ChannelID      uuid.UUID
	IGUserID       string
	TokenEnc       []byte
}

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	cipher, err := cryptoutil.NewTokenCipherFromBase64(cfg.Meta.TokenEncryptionKey)
	if err != nil {
		fatal(err)
	}
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		fatal(err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT c.id, c.team_id, p.id, p.provider_user_id, ch.id, COALESCE(ch.ig_user_id, ''), cc.access_token_enc
		FROM contacts c
		JOIN provider_profiles p
		  ON p.contact_id = c.id
		 AND p.provider = 'instagram'
		 AND p.deleted_at IS NULL
		JOIN LATERAL (
		  SELECT cv.channel_id
		  FROM conversations cv
		  WHERE cv.contact_id = c.id
		    AND cv.deleted_at IS NULL
		    AND cv.channel_type = 'instagram'
		    AND cv.channel_id IS NOT NULL
		  ORDER BY cv.last_message_at DESC NULLS LAST
		  LIMIT 1
		) conv ON true
		JOIN channels ch ON ch.id = conv.channel_id AND ch.deleted_at IS NULL
		JOIN channel_credentials cc ON cc.channel_id = ch.id
		WHERE c.deleted_at IS NULL
		  AND c.contact_name = 'Instagram User'
	`)
	if err != nil {
		fatal(err)
	}
	defer rows.Close()

	var targets []row
	for rows.Next() {
		var item row
		if err := rows.Scan(
			&item.ContactID, &item.TeamID, &item.ProfileID, &item.ProviderUserID,
			&item.ChannelID, &item.IGUserID, &item.TokenEnc,
		); err != nil {
			fatal(err)
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		fatal(err)
	}
	if len(targets) == 0 {
		fmt.Println("no Instagram User contacts left to enrich")
		return
	}

	client := metaclient.NewClient(cfg.Meta.APIVersion)
	updated := 0
	for _, item := range targets {
		token, err := cipher.Decrypt(item.TokenEnc)
		if err != nil {
			fmt.Printf("SKIP %s decrypt: %v\n", item.ProviderUserID, err)
			continue
		}

		name, username, avatar := "", "", ""
		if profile, err := client.GetInstagramScopedUser(ctx, item.ProviderUserID, token); err == nil && profile != nil {
			name = strings.TrimSpace(profile.Name)
			username = strings.TrimSpace(profile.Username)
			avatar = strings.TrimSpace(profile.ProfilePic)
		} else if err != nil {
			fmt.Printf("WARN %s public_profile: %v\n", item.ProviderUserID, err)
		}

		if (name == "" && username == "") && strings.TrimSpace(item.IGUserID) != "" {
			participant, err := client.GetInstagramConversationParticipant(ctx, item.IGUserID, item.ProviderUserID, token)
			if err != nil {
				fmt.Printf("SKIP %s conversations: %v\n", item.ProviderUserID, err)
				continue
			}
			name = strings.TrimSpace(participant.Name)
			username = strings.TrimSpace(participant.Username)
			fmt.Printf("FALLBACK %s conversations username=%q\n", item.ProviderUserID, username)
		}

		display := name
		if display == "" {
			display = username
		}
		if display == "" {
			fmt.Printf("SKIP %s empty profile\n", item.ProviderUserID)
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			fatal(err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE provider_profiles
			SET display_name = COALESCE(NULLIF($1, ''), display_name),
			    username = COALESCE(NULLIF($2, ''), username),
			    avatar_url = COALESCE(NULLIF($3, ''), avatar_url),
			    last_observed_at = NOW(),
			    updated_at = NOW()
			WHERE id = $4 AND team_id = $5
		`, name, username, avatar, item.ProfileID, item.TeamID); err != nil {
			_ = tx.Rollback(ctx)
			fmt.Printf("FAIL %s provider update: %v\n", item.ProviderUserID, err)
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE contacts
			SET contact_name = $1,
			    customer_name = $1,
			    avatar_url = COALESCE(NULLIF($2, ''), avatar_url),
			    updated_at = NOW()
			WHERE id = $3 AND team_id = $4 AND deleted_at IS NULL
		`, display, avatar, item.ContactID, item.TeamID); err != nil {
			_ = tx.Rollback(ctx)
			fmt.Printf("FAIL %s contact update: %v\n", item.ProviderUserID, err)
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			fmt.Printf("FAIL %s commit: %v\n", item.ProviderUserID, err)
			continue
		}
		updated++
		fmt.Printf("OK %s -> name=%q username=%q avatar=%v at=%s\n",
			item.ProviderUserID, display, username, avatar != "", time.Now().UTC().Format(time.RFC3339))
	}
	fmt.Printf("done: updated=%d total=%d\n", updated, len(targets))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
