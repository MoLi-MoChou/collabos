package service

import "testing"

func TestDetectProvider(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r.git": "github", "git@github.com:o/r.git": "github",
		"/srv/git/r.git": "local", "https://gitlab.com/o/r": "local", "https://github.com.evil/o/r": "local",
	} {
		if got := DetectProvider(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
}
