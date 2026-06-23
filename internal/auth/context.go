package auth

import (
	"context"
	"errors"
)

type ctxKey struct{}

var errActorMissing = errors.New("actor not found in context")

// WithActor stores the actor on a context (for logging or cross-cutting concerns).
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, actor)
}

// ActorFromContext returns the actor previously stored on the context.
func ActorFromContext(ctx context.Context) (Actor, error) {
	value := ctx.Value(ctxKey{})
	actor, ok := value.(Actor)
	if !ok {
		return Actor{}, errActorMissing
	}
	return actor, nil
}
