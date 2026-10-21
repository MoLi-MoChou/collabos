// Package logger configures slog and hands out component loggers.
package logger

import (
	"log/slog"
	"os"
)

// Init installs a text handler on stderr.
func Init(level slog.Level) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

// New returns a logger tagged with a component name.
func New(component string) *slog.Logger { return slog.Default().With("component", component) }
