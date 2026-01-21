package logger

import (
	"context"
	"log/slog"
	"os"
)

var Log *slog.Logger

func Init() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   "timestamp",
					Value: a.Value,
				}
			}
			if a.Key == slog.LevelKey {
				return slog.Attr{
					Key:   "level",
					Value: a.Value,
				}
			}
			if a.Key == slog.MessageKey {
				return slog.Attr{
					Key:   "message",
					Value: a.Value,
				}
			}
			return a
		},
	})
	Log = slog.New(handler)
}

func Info(msg string, args ...any) {
	Log.Info(msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	Log.InfoContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	Log.Error(msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	Log.ErrorContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	Log.Warn(msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	Log.WarnContext(ctx, msg, args...)
}

func Debug(msg string, args ...any) {
	Log.Debug(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	Log.DebugContext(ctx, msg, args...)
}
