// Package config loads server configuration from the environment once.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Server holds server settings.
type Server struct {
	Addr              string
	DatabaseURL       string
	AllowedOrigin     string
	DevLogin          bool
	HeartbeatInterval time.Duration
	OfflineGrace      time.Duration
	DefaultModel      string
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// LoadServer reads COLLAB_* variables and validates them.
func LoadServer() (Server, error) {
	hb, err := strconv.Atoi(env("COLLAB_HEARTBEAT_SECONDS", "15"))
	if err != nil {
		return Server{}, fmt.Errorf("config: COLLAB_HEARTBEAT_SECONDS: %w", err)
	}
	grace, err := time.ParseDuration(env("COLLAB_OFFLINE_GRACE", "10m"))
	if err != nil {
		return Server{}, fmt.Errorf("config: COLLAB_OFFLINE_GRACE: %w", err)
	}
	c := Server{
		Addr:              env("COLLAB_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		AllowedOrigin:     env("COLLAB_ALLOWED_ORIGIN", "http://localhost:5173"),
		DevLogin:          env("COLLAB_DEV_LOGIN", "true") == "true",
		HeartbeatInterval: time.Duration(hb) * time.Second,
		OfflineGrace:      grace,
		DefaultModel:      env("COLLAB_DEFAULT_MODEL", "opencode/deepseek-v4.1-flash-free"),
	}
	if c.DatabaseURL == "" {
		return Server{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	return c, nil
}
