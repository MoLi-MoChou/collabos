// Package config loads server configuration from the environment once.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Server holds server settings.
type Server struct {
	Addr          string
	DatabaseURL   string
	AllowedOrigin string
	DevLogin      bool
	// DevLoginPassword, when set, is a shared access code every dev login must present.
	DevLoginPassword string
	// DevLoginExistingOnly rejects emails that have no account yet (public previews).
	DevLoginExistingOnly bool
	HeartbeatInterval    time.Duration
	OfflineGrace         time.Duration
	DefaultModel         string
	// DecisionTimeout is how long intake, permission and merge decisions may
	// wait before the task is paused (never auto-approved) (PRD 3.2).
	DecisionTimeout time.Duration
	SweepInterval   time.Duration
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	// IMAllowHosts limits outbound IM webhooks (Slack/Feishu/DingTalk) to known hosts.
	IMAllowHosts []string
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
		Addr:                 env("COLLAB_ADDR", ":8080"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		AllowedOrigin:        env("COLLAB_ALLOWED_ORIGIN", "http://localhost:5173"),
		DevLogin:             env("COLLAB_DEV_LOGIN", "true") == "true",
		DevLoginPassword:     os.Getenv("COLLAB_DEV_LOGIN_PASSWORD"),
		DevLoginExistingOnly: env("COLLAB_DEV_LOGIN_EXISTING_ONLY", "false") == "true",
		HeartbeatInterval:    time.Duration(hb) * time.Second,
		OfflineGrace:         grace,
		DefaultModel:         env("COLLAB_DEFAULT_MODEL", "opencode/deepseek-v4.1-flash-free"),
	}
	c.DecisionTimeout = dur("COLLAB_DECISION_TIMEOUT", 24*time.Hour)
	c.SweepInterval = dur("COLLAB_SWEEP_INTERVAL", 30*time.Second)
	c.AccessTokenTTL = dur("COLLAB_ACCESS_TOKEN_TTL", 12*time.Hour)
	c.RefreshTokenTTL = dur("COLLAB_REFRESH_TOKEN_TTL", 30*24*time.Hour)
	c.IMAllowHosts = []string{"hooks.slack.com", "open.feishu.cn", "open.larksuite.com", "oapi.dingtalk.com"}
	for _, h := range strings.Split(os.Getenv("COLLAB_IM_ALLOW_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			c.IMAllowHosts = append(c.IMAllowHosts, h)
		}
	}
	if c.DatabaseURL == "" {
		return Server{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	return c, nil
}

func dur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// Defaults fills zero durations (tests build Server literals).
func (c *Server) Defaults() {
	if c.DecisionTimeout == 0 {
		c.DecisionTimeout = 24 * time.Hour
	}
	if c.SweepInterval == 0 {
		c.SweepInterval = 30 * time.Second
	}
	if c.AccessTokenTTL == 0 {
		c.AccessTokenTTL = 12 * time.Hour
	}
	if c.RefreshTokenTTL == 0 {
		c.RefreshTokenTTL = 30 * 24 * time.Hour
	}
}
