package rbac

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/omnichannel/omni-chan-v2/pkg/database"
)

var ErrNotFound = errors.New("role not found")

type Repository interface {
	LoadRolePermissions(ctx context.Context) (map[string]map[string]bool, error)
	ListRoles(ctx context.Context) ([]Role, error)
	FindRoleByID(ctx context.Context, id uuid.UUID) (*Role, error)
	FindRoleByName(ctx context.Context, name string) (*Role, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	ListPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]Permission, error)
	GetPermissionMatrix(ctx context.Context) ([]PermissionMatrixRow, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) LoadRolePermissions(ctx context.Context) (map[string]map[string]bool, error) {
	query := `
		SELECT ro.name, p.code
		FROM role_permissions rp
		INNER JOIN roles ro ON ro.id = rp.role_id
		INNER JOIN permissions p ON p.id = rp.permission_id
		WHERE ro.deleted_at IS NULL
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("load role permissions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]bool)
	for rows.Next() {
		var roleName, code string
		if err := rows.Scan(&roleName, &code); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		if result[roleName] == nil {
			result[roleName] = make(map[string]bool)
		}
		result[roleName][code] = true
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ListRoles(ctx context.Context) ([]Role, error) {
	query := `
		SELECT id, name, description, is_system, created_at, updated_at
		FROM roles
		WHERE deleted_at IS NULL
		ORDER BY name ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *PostgresRepository) FindRoleByID(ctx context.Context, id uuid.UUID) (*Role, error) {
	query := `
		SELECT id, name, description, is_system, created_at, updated_at
		FROM roles
		WHERE id = $1 AND deleted_at IS NULL
	`
	var role Role
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find role: %w", err)
	}
	return &role, nil
}

func (r *PostgresRepository) FindRoleByName(ctx context.Context, name string) (*Role, error) {
	query := `
		SELECT id, name, description, is_system, created_at, updated_at
		FROM roles
		WHERE LOWER(name) = LOWER($1) AND deleted_at IS NULL
	`
	var role Role
	err := r.db.Pool.QueryRow(ctx, query, name).Scan(
		&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find role by name: %w", err)
	}
	return &role, nil
}

func (r *PostgresRepository) ListPermissions(ctx context.Context) ([]Permission, error) {
	query := `
		SELECT id, code, name, description
		FROM permissions
		ORDER BY code ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

func (r *PostgresRepository) ListPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]Permission, error) {
	query := `
		SELECT p.id, p.code, p.name, p.description
		FROM role_permissions rp
		INNER JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1
		ORDER BY p.code ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions by role: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

func (r *PostgresRepository) GetPermissionMatrix(ctx context.Context) ([]PermissionMatrixRow, error) {
	permissions, err := r.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := r.ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	grants := make(map[string]map[string]bool)
	for _, role := range roles {
		perms, err := r.ListPermissionsByRoleID(ctx, role.ID)
		if err != nil {
			return nil, err
		}
		grants[role.Name] = make(map[string]bool)
		for _, p := range perms {
			grants[role.Name][p.Code] = true
		}
	}

	rows := make([]PermissionMatrixRow, 0, len(permissions))
	for _, p := range permissions {
		roleMap := make(map[string]bool, len(roles))
		for _, role := range roles {
			roleMap[role.Name] = grants[role.Name][p.Code]
		}
		rows = append(rows, PermissionMatrixRow{
			Permission: p,
			Roles:      roleMap,
		})
	}
	return rows, nil
}
