package rbac

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPermissionDenied     = ErrNotFound // re-export pattern; handlers use message
	ErrForbidden            = errors.New("forbidden")
	ErrOwnerRoleImmutable   = errors.New("cannot modify owner role permissions")
	ErrSystemRoleDelete     = errors.New("cannot delete system role")
	ErrSystemRoleNameChange = errors.New("cannot rename system role")
)

type Service struct {
	repo  Repository
	mu    sync.RWMutex
	cache map[string]map[string]bool
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) WarmCache(ctx context.Context) error {
	cache, err := s.repo.LoadRolePermissions(ctx)
	if err != nil {
		// Allow bootstrapping environments where RBAC migration is not applied yet.
		if strings.Contains(strings.ToLower(err.Error()), `relation "role_permissions" does not exist`) {
			s.mu.Lock()
			s.cache = map[string]map[string]bool{}
			s.mu.Unlock()
			return nil
		}
		return err
	}
	s.mu.Lock()
	s.cache = cache
	s.mu.Unlock()
	return nil
}

func (s *Service) HasPermission(_ context.Context, roleName, code string) (bool, error) {
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cache == nil {
		return false, nil
	}
	perms, ok := s.cache[roleName]
	if !ok {
		return false, nil
	}
	return perms[code], nil
}

func (s *Service) Require(roleName, code string) bool {
	ok, _ := s.HasPermission(context.Background(), roleName, code)
	return ok
}

// PermissionsForRole returns a copy of the permission snapshot for a role name.
func (s *Service) PermissionsForRole(roleName string) map[string]bool {
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cache == nil {
		return map[string]bool{}
	}
	perms, ok := s.cache[roleName]
	if !ok {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(perms))
	for code, granted := range perms {
		out[code] = granted
	}
	return out
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *Service) GetRole(ctx context.Context, roleID string) (*RoleDetail, error) {
	id, err := parseUUID(roleID)
	if err != nil {
		return nil, err
	}
	role, err := s.repo.FindRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	perms, err := s.repo.ListPermissionsByRoleID(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	return &RoleDetail{Role: *role, Permissions: perms}, nil
}

func (s *Service) GetPermissionMatrix(ctx context.Context) ([]PermissionMatrixRow, error) {
	return s.repo.GetPermissionMatrix(ctx)
}

func (s *Service) FindRoleByName(ctx context.Context, name string) (*Role, error) {
	return s.repo.FindRoleByName(ctx, name)
}

func (s *Service) authorizeManage(actorRole string) error {
	if !s.Require(actorRole, PermTeamManage) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) CreateRole(ctx context.Context, actorRole, name, description string) (*Role, error) {
	if err := s.authorizeManage(actorRole); err != nil {
		return nil, err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	description = strings.TrimSpace(description)
	if name == "" || description == "" {
		return nil, errors.New("name and description are required")
	}
	now := time.Now().UTC()
	role := &Role{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		IsSystem:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *Service) UpdateRoleMeta(ctx context.Context, actorRole, roleID, name, description string) (*Role, error) {
	if err := s.authorizeManage(actorRole); err != nil {
		return nil, err
	}
	id, err := parseUUID(roleID)
	if err != nil {
		return nil, err
	}
	role, err := s.repo.FindRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, errors.New("description is required")
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if role.IsSystem {
		if name != "" && !strings.EqualFold(name, role.Name) {
			return nil, ErrSystemRoleNameChange
		}
		role.Description = description
	} else {
		if name == "" {
			return nil, errors.New("name is required")
		}
		role.Name = name
		role.Description = description
	}
	role.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *Service) DeleteRole(ctx context.Context, actorRole, roleID string) error {
	if err := s.authorizeManage(actorRole); err != nil {
		return err
	}
	id, err := parseUUID(roleID)
	if err != nil {
		return err
	}
	role, err := s.repo.FindRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return ErrSystemRoleDelete
	}
	if err := s.repo.SoftDeleteRole(ctx, id); err != nil {
		return err
	}
	return s.WarmCache(ctx)
}

func (s *Service) UpdateRolePermissions(ctx context.Context, actorRole, roleID string, permissionCodes []string) error {
	if err := s.authorizeManage(actorRole); err != nil {
		return err
	}
	id, err := parseUUID(roleID)
	if err != nil {
		return err
	}
	role, err := s.repo.FindRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if strings.EqualFold(role.Name, "owner") {
		return ErrOwnerRoleImmutable
	}
	normalized := make([]string, 0, len(permissionCodes))
	seen := make(map[string]struct{}, len(permissionCodes))
	for _, code := range permissionCodes {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	if err := s.repo.ReplaceRolePermissions(ctx, id, normalized); err != nil {
		return err
	}
	return s.WarmCache(ctx)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
