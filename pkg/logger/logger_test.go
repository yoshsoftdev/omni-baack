package logger

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	log := New("development")
	if log == nil {
		t.Fatal("expected logger")
	}
	log.Info("test message")
}

func TestWithContext(t *testing.T) {
	log := New("production")
	ctx := WithContext(context.Background(), log)

	got := FromContext(ctx)
	if got != log {
		t.Error("FromContext should return injected logger")
	}
}

func TestFromContext_Default(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Fatal("expected default logger")
	}
}
