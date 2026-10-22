package runner

import (
	"reflect"
	"testing"
)

func TestShellArgvPerPlatform(t *testing.T) {
	if got := shellArgv("linux", "go test ./..."); !reflect.DeepEqual(got, []string{"sh", "-c", "go test ./..."}) {
		t.Fatalf("linux: %v", got)
	}
	if got := shellArgv("windows", "npm test"); !reflect.DeepEqual(got, []string{"cmd.exe", "/d", "/s", "/c", "npm test"}) {
		t.Fatalf("windows: %v", got)
	}
}
