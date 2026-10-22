package config

import "testing"

func TestNormalizeRemote(t *testing.T) {
	a := NormalizeRemote("git@github.com:Acme/Demo.git")
	for _, r := range []string{"https://github.com/acme/demo", "https://github.com/acme/demo.git", "ssh://git@github.com/acme/demo.git", "https://user@github.com/acme/demo/"} {
		if NormalizeRemote(r) != a {
			t.Fatalf("%s -> %s, want %s", r, NormalizeRemote(r), a)
		}
	}
	c := Config{Repos: map[string]string{"git@github.com:acme/demo.git": "/src/demo"}}
	if c.RepoFor("https://github.com/acme/demo") != "/src/demo" || c.RepoFor("") != "" {
		t.Fatal("RepoFor")
	}
}

func TestAgentPresets(t *testing.T) {
	if got := (Config{}).AgentArgv(); got[0] != "opencode" || got[1] != "acp" {
		t.Fatal(got)
	}
	if got := (Config{Agent: []string{"claude"}}).AgentArgv(); got[0] != "claude-agent-acp" {
		t.Fatal(got)
	}
	if got := (Config{Agent: []string{"my-agent", "--acp"}}).AgentArgv(); len(got) != 2 {
		t.Fatal(got)
	}
}
