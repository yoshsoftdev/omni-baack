package report

import (
	"time"

	"github.com/google/uuid"
)

type DashboardStats struct {
	TotalCampaigns int
	MessagesSent   int
	MessagesDelivered int
	MessagesRead   int
	MessagesFailed int
	RepliesReceived int
	LinkClicks     int
}

type DailyStat struct {
	ID                uuid.UUID
	TeamID            uuid.UUID
	StatDate          time.Time
	CampaignsStarted  int
	MessagesSent      int
	MessagesDelivered int
	MessagesRead      int
	MessagesFailed    int
	RepliesReceived   int
	LinkClicks        int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CampaignFunnel struct {
	CampaignID      uuid.UUID
	TeamID          uuid.UUID
	Name            string
	Status          string
	TotalContacts   int
	Sent            int
	Delivered       int
	Read            int
	Failed          int
	Replies         int
	Clicked         int
	StartedAt       *time.Time
	CompletedAt     *time.Time
}

type LinkClick struct {
	ID          uuid.UUID
	CampaignID  uuid.UUID
	RecipientID *uuid.UUID
	TeamID      uuid.UUID
	URL         string
	ClickedAt   time.Time
	UserAgent   *string
	DeviceType  *string
}

type DateRange struct {
	From time.Time
	To   time.Time
}

type DailyStatDelta struct {
	CampaignsStarted  int
	MessagesSent      int
	MessagesDelivered int
	MessagesRead      int
	MessagesFailed    int
	RepliesReceived   int
	LinkClicks        int
}
