// Package config is the per-device client configuration, shared by
// `collab daemon` and `collab mcp`. It lives in COLLAB_HOME/config.json
// (default ~/.collab).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the device configuration.
type Config struct {
	Server string `json:"server"`
	Token  string `json:"token"`
	// Agent is a preset name (opencode, claude, codex, gemini, grok) or a full
	// command line split into argv, e.g. ["npx","-y","@zed-industries/claude-code-acp"].
	Agent []string `json:"agent,omitempty"`
	// Model is passed to opencode (provider/model).
	Model string `json:"model,omitempty"`
	// Env is added to the agent's environment.
	Env map[string]string `json:"env,omitempty"`
	// Repos maps a git remote URL to a local clone, so callees can check out
	// the caller's code.
	Repos map[string]string `json:"repos,omitempty"`
	// Inactivity fails a call when the agent sends nothing for this long
	// (Go duration, default "5m", "0" disables). COLLAB_AGENT_INACTIVITY overrides.
	Inactivity string `json:"inactivity,omitempty"`
	// MaxConcurrent caps simultaneous incoming calls (default 2).
	MaxConcurrent int `json:"max_concurrent,omitempty"`
}

// Presets map agent names to ACP commands.
var Presets = map[string][]string{
	"opencode": {"opencode", "acp"},
	"claude":   {"claude-agent-acp"},
	"codex":    {"codex-acp"},
	"gemini":   {"gemini", "--experimental-acp"},
	"grok":     {"grok", "acp"},
}

// Home is the config directory.
func Home() string {
	if h := os.Getenv("COLLAB_HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".collab")
}

// Path is the config file.
func Path() string { return filepath.Join(Home(), "config.json") }

// Load reads the config; COLLAB_SERVER and COLLAB_TOKEN override it.
func Load() (Config, error) {
	var c Config
	b, err := os.ReadFile(Path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &c); err != nil {
			return c, err
		}
	}
	if v := os.Getenv("COLLAB_SERVER"); v != "" {
		c.Server = v
	}
	if v := os.Getenv("COLLAB_TOKEN"); v != "" {
		c.Token = v
	}
	c.Server = strings.TrimRight(c.Server, "/")
	return c, nil
}

// Save writes the config (0600: it holds the device token).
func Save(c Config) error {
	if err := os.MkdirAll(Home(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(Path(), b, 0o600)
}

// InactivityTimeout is the parsed inactivity limit.
func (c Config) InactivityTimeout() time.Duration {
	v := c.Inactivity
	if e := os.Getenv("COLLAB_AGENT_INACTIVITY"); e != "" {
		v = e
	}
	if v == "" {
		return 5 * time.Minute
	}
	if v == "0" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 5 * time.Minute
	}
	return d
}

// AgentArgv resolves the agent command.
func (c Config) AgentArgv() []string {
	if len(c.Agent) == 0 {
		return Presets["opencode"]
	}
	if len(c.Agent) == 1 {
		if p, ok := Presets[c.Agent[0]]; ok {
			return p
		}
	}
	return c.Agent
}

// NormalizeRemote makes https and ssh spellings of one remote comparable.
func NormalizeRemote(r string) string {
	r = strings.TrimSpace(strings.ToLower(r))
	r = strings.TrimSuffix(strings.TrimSuffix(r, "/"), ".git")
	for _, p := range []string{"https://", "http://", "ssh://", "git://"} {
		r = strings.TrimPrefix(r, p)
	}
	r = strings.TrimPrefix(r, "git@")
	if i := strings.Index(r, "@"); i >= 0 && i < strings.Index(r+"/", "/") {
		r = r[i+1:]
	}
	return strings.Replace(r, ":", "/", 1)
}

// RepoFor finds the local clone of remote.
func (c Config) RepoFor(remote string) string {
	if remote == "" {
		return ""
	}
	n := NormalizeRemote(remote)
	for k, v := range c.Repos {
		if NormalizeRemote(k) == n || k == remote {
			return v
		}
	}
	return ""
}
