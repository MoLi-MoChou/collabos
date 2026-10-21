// Package config stores the daemon's local configuration under COLLAB_HOME
// (default ~/.collabos): server URL, node token and the repo map.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
