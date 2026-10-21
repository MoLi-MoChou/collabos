package handler

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every /api route registered in handler.go must be documented in openapi.yaml.
func TestOpenAPICoversRoutes(t *testing.T) {
	src, _ := os.ReadFile("handler.go")
	spec, _ := os.ReadFile("openapi.yaml")
	re := regexp.MustCompile(`r\.(Get|Post|Put|Patch|Delete)\("(/api[^"]*)"`)
	n := 0
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		n++
		if !strings.Contains(string(spec), "\n  "+m[2]+":\n") {
			t.Errorf("%s %s missing from openapi.yaml", m[1], m[2])
		}
	}
	if n < 50 {
		t.Fatalf("only %d routes found", n)
	}
}
