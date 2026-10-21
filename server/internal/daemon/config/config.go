// Package config stores the daemon's local configuration under COLLAB_HOME
// (default ~/.collabos): server URL, node token and the repo map.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo maps a project remote to a local checkout.
type Repo struct {
	RemoteURL string `json:"remote_url"`
	Path      string `json:"path"`
}

// Config is the persisted daemon configuration.
type Config struct {
	ServerURL string `json:"server_url"`
	NodeToken string `json:"node_token"`
	DaemonID  string `json:"daemon_id,omitempty"`
	Repos     []Repo `json:"repos"`
	// OpencodeBin overrides the opencode binary path.
	OpencodeBin string `json:"opencode_bin,omitempty"`
	// EgressAllow adds hosts the sandboxed agent may reach (see Egress()).
	EgressAllow []string `json:"egress_allow,omitempty"`
	// EgressOff disables the egress gate: the sandbox shares the host network.
	EgressOff bool `json:"egress_off,omitempty"`
}

// DefaultEgressAllow are the model API hosts opencode needs by default.
var DefaultEgressAllow = []string{"opencode.ai", "*.opencode.ai", "models.dev"}

// Egress returns whether the gate is on and the effective allowlist.
// COLLAB_EGRESS=off disables it; COLLAB_EGRESS_ALLOW (comma separated) adds hosts.
func (c Config) Egress() (bool, []string) {
	if c.EgressOff || os.Getenv("COLLAB_EGRESS") == "off" {
		return false, nil
	}
	allow := append(append([]string{}, DefaultEgressAllow...), c.EgressAllow...)
	for _, h := range strings.Split(os.Getenv("COLLAB_EGRESS_ALLOW"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			allow = append(allow, h)
		}
	}
	return true, allow
}

// TaskLimits reads per-task resource caps (FR-DAE-103):
// COLLAB_TASK_CPU_SECONDS, COLLAB_TASK_MEM_MB, COLLAB_TASK_MAX_PROCS. Unset means no cap.
func TaskLimits() (cpu, memMB, procs int) {
	n := func(k string) int {
		v, _ := strconv.Atoi(os.Getenv(k))
		return v
	}
	return n("COLLAB_TASK_CPU_SECONDS"), n("COLLAB_TASK_MEM_MB"), n("COLLAB_TASK_MAX_PROCS")
}

// AgentInactivityTimeout is how long a running agent may go without any
// ACP traffic (outside human waits) before the task fails with
// agent_inactive. COLLAB_AGENT_INACTIVITY_TIMEOUT, Go duration; default 10m,
// "0" disables.
func AgentInactivityTimeout() time.Duration {
	v := os.Getenv("COLLAB_AGENT_INACTIVITY_TIMEOUT")
	if v == "" {
		return 10 * time.Minute
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 10 * time.Minute
	}
	return d
}

// Home returns the daemon home directory.
func Home() string {
	if h := os.Getenv("COLLAB_HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".collabos")
}

// Path is the config file path.
func Path() string { return filepath.Join(Home(), "daemon.json") }

// Load reads the config (empty config when missing).
func Load() (Config, error) {
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	return c, json.Unmarshal(b, &c)
}

// Save writes the config with 0600 permissions (it holds the node token).
func Save(c Config) error {
	if err := os.MkdirAll(Home(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(Path(), b, 0o600)
}

// RepoFor finds the local checkout of a remote.
func (c Config) RepoFor(remote string) (string, bool) {
	for _, r := range c.Repos {
		if r.RemoteURL == remote {
			return r.Path, true
		}
	}
	return "", false
}

// AddRepo inserts or replaces a repo mapping.
func (c *Config) AddRepo(r Repo) {
	for i := range c.Repos {
		if c.Repos[i].RemoteURL == r.RemoteURL {
			c.Repos[i] = r
			return
		}
	}
	c.Repos = append(c.Repos, r)
}
