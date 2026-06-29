//go:build integration

package contact_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

func TestUpdateContactIntegration(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, err := database.New(ctx, cfg.Database)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	teamRepo := team.NewRepository(db)
	contactRepo := contact.NewRepository(db)
	rbacSvc := rbac.NewService(rbac.NewRepository(db))
	_ = rbacSvc.WarmCache(ctx)
	svc := contact.NewService(contactRepo, teamRepo, nil, rbacSvc)

	teamID := uuid.MustParse("299582b8-4db5-4fdc-b6ee-86e993b425c9")
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	contactID := uuid.MustParse("46560d1b-27f3-466e-8ea7-3d2b8196da0d")

	members, err := teamRepo.ListMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, member := range members {
		if member.Role == team.RoleOwner {
			userID = member.UserID
			break
		}
	}

	name := "YoShandy"
	phone := "+6282288812003"
	province := "riau"
	company := "dgos"
	marketing := contact.MarketingSubscribed
	tagID := uuid.MustParse("8efb611a-ed65-4a3a-bec0-de20698509ba")

	updated, err := svc.Update(ctx, teamID, userID, string(team.RoleOwner), contactID, contact.UpdateInput{
		Name:            &name,
		Phone:           &phone,
		Province:        &province,
		Company:         &company,
		MarketingStatus: &marketing,
		TagIDs:          []uuid.UUID{tagID},
		TagIDsSet:       true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated == nil {
		t.Fatal("Update() returned nil contact")
	}

	longCountry := "INDONESIA"
	if _, err := svc.Update(ctx, teamID, userID, string(team.RoleOwner), contactID, contact.UpdateInput{
		Country: &longCountry,
	}); err == nil {
		t.Fatal("expected validation error for long country")
	}
}
