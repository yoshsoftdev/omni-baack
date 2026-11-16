package meta

import (
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

// Authorizer checks a role's permission (rbac.Service). Without one, every check fails closed.
type Authorizer interface {
	Require(role, code string) bool
}

func authorize(a Authorizer, role, perm string) error {
	if a != nil && a.Require(role, perm) {
		return nil
	}
	return serviceerrors.ErrForbidden
}

// Permissions used by the channel-facing Meta services (TEAM-04).
const (
	permChannelView   = rbac.PermDeviceView
	permChannelManage = rbac.PermDeviceManage
)
