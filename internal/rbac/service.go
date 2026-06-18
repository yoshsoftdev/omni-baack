package rbac

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var ErrPermissionDenied = ErrNotFound // re-export pattern; handlers use message

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

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
