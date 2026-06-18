package rbac

import (
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
	IsSystem    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Permission struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type RolePermission struct {
	RoleID       uuid.UUID
	PermissionID uuid.UUID
}

type PermissionMatrixRow struct {
	Permission Permission `json:"permission"`
	Roles      map[string]bool `json:"roles"`
}

type RoleDetail struct {
	Role        Role         `json:"role"`
	Permissions []Permission `json:"permissions"`
}
