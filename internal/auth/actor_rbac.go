package auth

import "github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"

// HasPermission reports whether the actor has the given permission code.
func (a Actor) HasPermission(code string) bool {
	return a.Permissions.Has(code)
}

// RequirePermission returns nil when the actor has the permission.
func (a Actor) RequirePermission(code string) error {
	if !a.HasPermission(code) {
		return serviceerrors.ErrForbidden
	}
	return nil
}
