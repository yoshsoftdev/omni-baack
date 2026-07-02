package channelsync

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PermissionState string

const (
	PermissionGranted PermissionState = "granted"
	PermissionMissing PermissionState = "missing"
	PermissionExpired PermissionState = "expired"
)

type SyncStatus string

const (
	SyncNotSynced  SyncStatus = "not_synced"
	SyncInProgress SyncStatus = "in_progress"
	SyncCompleted  SyncStatus = "completed"
	SyncFailed     SyncStatus = "failed"
)

type Permissions struct {
	Contacts         bool       `json:"contacts"`
	Templates        bool       `json:"templates"`
	ConversationSync bool       `json:"conversation_sync"`
	LastCheckedAt    *time.Time `json:"last_checked_at,omitempty"`
}

type State struct {
	ChannelID              uuid.UUID
	Permissions            Permissions
	ContactsStatus         SyncStatus
	ContactsCreated        int
	ContactsUpdated        int
	TemplatesStatus        SyncStatus
	TemplatesSynced        int
	ConversationsStatus    SyncStatus
	ConversationsImported  int
	ConversationsReason    *string
	ContactsSyncRequestID  *string
	HistorySyncRequestID   *string
	LastSyncAt             *time.Time
	ContactsLastSyncAt     *time.Time
	TemplatesLastSyncAt    *time.Time
	ConversationsLastSyncAt *time.Time
	LastPermissionCheckAt  *time.Time
	UpdatedAt              time.Time
}

type ContactEvent struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	TeamID      uuid.UUID
	Action      string
	PhoneNumber string
	FullName    string
	FirstName   string
	CreatedAt   time.Time
}

type ContactSyncResult struct {
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Duplicates int `json:"duplicates"`
}

type ConversationSyncResult struct {
	Imported int     `json:"imported"`
	Reason   *string `json:"reason,omitempty"`
}

type ResourceStatus struct {
	Status  SyncStatus `json:"status"`
	Created *int       `json:"created,omitempty"`
	Updated *int       `json:"updated,omitempty"`
	Synced  *int       `json:"synced,omitempty"`
	Imported *int      `json:"imported,omitempty"`
}

type StatusResponse struct {
	Permissions   PermissionState `json:"permissions"`
	Contacts      ResourceStatus  `json:"contacts"`
	Templates     ResourceStatus  `json:"templates"`
	Conversations ResourceStatus  `json:"conversations"`
	LastSyncAt    *time.Time      `json:"last_sync_at,omitempty"`
}

func defaultPermissions() Permissions {
	return Permissions{Templates: true}
}

func permissionsFromJSON(raw json.RawMessage) Permissions {
	if len(raw) == 0 {
		return defaultPermissions()
	}
	var perms Permissions
	if err := json.Unmarshal(raw, &perms); err != nil {
		return defaultPermissions()
	}
	if !perms.Templates {
		perms.Templates = true
	}
	return perms
}

func (p Permissions) MarshalJSON() ([]byte, error) {
	type alias Permissions
	return json.Marshal(alias(p))
}

func aggregatePermissionState(perms Permissions, tokenExpired bool) PermissionState {
	if tokenExpired {
		return PermissionExpired
	}
	if perms.Contacts && perms.Templates && perms.ConversationSync {
		return PermissionGranted
	}
	if !perms.Contacts && !perms.ConversationSync {
		return PermissionMissing
	}
	if perms.Templates {
		return PermissionGranted
	}
	return PermissionMissing
}
