package approval

import (
	"testing"

	p "github.com/collabos/collabos/server/pkg/protocol"
)

func TestPolicy(t *testing.T) {
	pol := Default()
	root := "/r/wt"
	g := p.Grant{Resources: []string{"src/**"}, Actions: []string{"read", "edit", "execute"}}
	cases := []struct {
		req  p.PermissionRequest
		want Verdict
		code string
	}{
		{p.PermissionRequest{Kind: "edit", Paths: []string{"/r/wt/src/a.txt"}}, Allow, ""},
		{p.PermissionRequest{Kind: "edit", Paths: []string{"/r/wt/README.md"}}, Escalate, "scope_exceeded"},
		{p.PermissionRequest{Kind: "edit", Paths: []string{"/r/other/a.txt"}}, Deny, "scope_exceeded"},
		{p.PermissionRequest{Kind: "edit", Paths: []string{"/r/wt/../x"}}, Deny, "scope_exceeded"},
		{p.PermissionRequest{Kind: "execute", Command: "ls -la"}, Allow, ""},
		{p.PermissionRequest{Kind: "execute", Command: "sudo rm x"}, Deny, "action_forbidden"},
		{p.PermissionRequest{Kind: "execute", Command: "git push origin main"}, Deny, "action_forbidden"},
		{p.PermissionRequest{Kind: "fetch"}, Escalate, "action_forbidden"},
	}
	for i, c := range cases {
		r := pol.Decide(g, c.req, root)
		if r.Verdict != c.want || r.Code != c.code {
			t.Errorf("case %d: got %s/%s (%s), want %s/%s", i, r.Verdict, r.Code, r.Reason, c.want, c.code)
		}
	}
	w := Widen(g, []string{"README.md"}, "")
	if r := pol.Decide(w, p.PermissionRequest{Kind: "edit", Paths: []string{"/r/wt/README.md"}}, root); r.Verdict != Allow {
		t.Fatalf("widened grant should allow: %+v", r)
	}
	if len(g.Resources) != 1 {
		t.Fatal("Widen must not mutate input")
	}
}
