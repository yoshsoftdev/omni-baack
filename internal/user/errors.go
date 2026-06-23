package user

import "github.com/omnichannel/omni-chan-v2/pkg/repository"

// ErrNotFound is returned when a user record does not exist.
var ErrNotFound = repository.ErrNotFound

// ErrEmailTaken is returned when registering with an email that already exists.
var ErrEmailTaken = repository.ErrConflict
