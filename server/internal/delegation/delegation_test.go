package delegation

import (
	"testing"

	p "github.com/collabos/collabos/server/pkg/protocol"
)

func TestMatchResource(t *testing.T) {
	cases := []struct {
		pat, rel string
		ok       bool
	}{
		{"**", "a/b.go", true},
		{"src/**", "src/login/a.ts", true},
		{"src/**", "src", true},
		{"src/**", "srcx/a", false},
		{"src/**", "docs/a.md", false},
		{"README.md", "README.md", true},
		{"*.md", "a.md", true},
		{"src/**", "src/../docs/x", false},
	}
	for _, c := range cases {
		if got := MatchResource(c.pat, c.rel); got != c.ok {
			t.Errorf("%s ~ %s = %v", c.pat, c.rel, got)
		}
	}
}

func TestIsSubset(t *testing.T) {
	parent := p.Grant{Resources: []string{"src/**"}, Actions: []string{"read", "edit"}, Network: []string{"npm"}, ExpiresAt: "2026-10-22T18:00:00+08:00"}
	ok := p.Grant{Resources: []string{"src/login/**", "src/a.ts"}, Actions: []string{"read"}, ExpiresAt: "2026-10-22T17:00:00+08:00"}
	if !IsSubset(ok, parent) {
		t.Fatal("expected subset")
	}
	bad := []p.Grant{
		{Resources: []string{"**"}, Actions: []string{"read"}, ExpiresAt: parent.ExpiresAt},
		{Resources: []string{"docs/**"}, Actions: []string{"read"}, ExpiresAt: parent.ExpiresAt},
		{Resources: []string{"src/**"}, Actions: []string{"execute"}, ExpiresAt: parent.ExpiresAt},
		{Resources: []string{"src/**"}, Actions: []string{"read"}, Network: []string{"github"}, ExpiresAt: parent.ExpiresAt},
		{Resources: []string{"src/**"}, Actions: []string{"read"}, ExpiresAt: "2026-10-23T00:00:00+08:00"},
		{Resources: []string{"src/**"}, Actions: []string{"read"}},
	}
	for i, g := range bad {
		if IsSubset(g, parent) {
			t.Errorf("case %d should widen parent", i)
		}
	}
}

func TestValidate(t *testing.T) {
	g := p.Grant{Resources: []string{"src/**"}, Actions: []string{"read"}}
	if err := Validate(Request{Grant: g, Depth: 4, MaxDepth: 3}, nil); err == nil || err.Code != CodeDepthExceeded {
		t.Fatalf("depth: %v", err)
	}
	if err := Validate(Request{Grant: p.Grant{}, Depth: 1}, nil); err == nil || err.Code != CodeScopeExceeded {
		t.Fatalf("empty grant: %v", err)
	}
	par := &Parent{Grant: p.Grant{Resources: []string{"src/login/**"}, Actions: []string{"read"}}, Budget: p.Budget{MaxTokens: 1000}, UsedTokens: 800, MaxDepth: 2}
	if err := Validate(Request{Grant: g, Depth: 2, Budget: p.Budget{MaxTokens: 100}}, par); err == nil || err.Code != CodeScopeExceeded {
		t.Fatalf("subset: %v", err)
	}
	sub := p.Grant{Resources: []string{"src/login/a.ts"}, Actions: []string{"read"}}
	if err := Validate(Request{Grant: sub, Depth: 2, Budget: p.Budget{MaxTokens: 500}}, par); err == nil || err.Code != CodeBudgetExceeded {
		t.Fatalf("budget: %v", err)
	}
	if err := Validate(Request{Grant: sub, Depth: 3, MaxDepth: 5, Budget: p.Budget{MaxTokens: 100}}, par); err == nil || err.Code != CodeDepthExceeded {
		t.Fatalf("max depth inherited: %v", err)
	}
	if err := Validate(Request{Grant: sub, Depth: 2, Budget: p.Budget{MaxTokens: 100}}, par); err != nil {
		t.Fatalf("valid: %v", err)
	}
}

func TestDecideIntake(t *testing.T) {
	cases := []struct {
		name string
		pol  IntakePolicy
		in   IntakeInput
		want IntakeOutcome
		code string
	}{
		{"self", IntakePolicy{Mode: IntakeClosed}, IntakeInput{RequesterID: "a", OwnerID: "a", Sandbox: p.SandboxNone}, IntakeAccept, ""},
		{"no sandbox", IntakePolicy{Mode: IntakeManual}, IntakeInput{RequesterID: "a", OwnerID: "b", Sandbox: p.SandboxNone}, IntakeReject, CodeSandboxUnavailable},
		{"closed", IntakePolicy{Mode: IntakeClosed}, IntakeInput{RequesterID: "a", OwnerID: "b", Sandbox: p.SandboxBubblewrap}, IntakeReject, CodeIntakeDenied},
		{"quota", IntakePolicy{Mode: IntakeManual, DailyQuota: 2}, IntakeInput{RequesterID: "a", OwnerID: "b", AcceptedToday: 2, Sandbox: p.SandboxBubblewrap}, IntakeReject, CodeQuotaExceeded},
		{"allowlisted", IntakePolicy{Mode: IntakeAllowlist, Allowlist: []string{"a"}}, IntakeInput{RequesterID: "a", OwnerID: "b", Sandbox: p.SandboxBubblewrap}, IntakeAccept, ""},
		{"not allowlisted", IntakePolicy{Mode: IntakeAllowlist, Allowlist: []string{"c"}}, IntakeInput{RequesterID: "a", OwnerID: "b", Sandbox: p.SandboxBubblewrap}, IntakePending, ""},
		{"manual", IntakePolicy{Mode: IntakeManual}, IntakeInput{RequesterID: "a", OwnerID: "b", Sandbox: p.SandboxSeatbelt}, IntakePending, ""},
	}
	for _, c := range cases {
		got, code := DecideIntake(c.pol, c.in)
		if got != c.want || code != c.code {
			t.Errorf("%s: got %s/%s want %s/%s", c.name, got, code, c.want, c.code)
		}
	}
}
