package rbac

import (
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
