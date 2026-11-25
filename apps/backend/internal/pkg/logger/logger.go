package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the process-wide slog handler. Levels are controlled with
// LOG_LEVEL (debug|info|warn|error); anything unset means info.
func Init() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }
func Debug(msg string, args ...any) { slog.Debug(msg, args...) }

func Fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}
