package logger

import (
	"context"
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
}

func New(env string) *Logger {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return &Logger{Logger: slog.New(handler)}
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

func FromContext(ctx context.Context) *Logger {
	if v := ctx.Value(loggerKey{}); v != nil {
		if log, ok := v.(*Logger); ok {
			return log
		}
	}
	return New("development")
}

func WithContext(ctx context.Context, log *Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, log)
}

type loggerKey struct{}
