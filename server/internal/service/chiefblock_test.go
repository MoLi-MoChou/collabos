package service

import "testing"

func TestParseDelegationBlock(t *testing.T) {
	for _, tc := range []struct {
		name, in      string
		ok, found     bool
		target, clean string
	}{
		{"valid", "先说明\n```delegation\n{\"target\":\"@LS-Coder\",\"title\":\"t\",\"prompt\":\"p\"}\n```", true, true, "ls-coder", "先说明"},
		{"upper fence", "```Delegation\n{\"target\":\"x\",\"title\":\"t\"}\n```", true, true, "x", ""},
		{"malformed", "好\n```delegation\n{target: x}\n```", false, true, "", "好\n```delegation\n{target: x}\n```"},
		{"unterminated", "```delegation\n{\"target\":\"x\"", false, true, "", "```delegation\n{\"target\":\"x\""},
		{"none", "只是回答", false, false, "", "只是回答"},
		{"missing title", "```delegation\n{\"target\":\"x\"}\n```", false, true, "x", ""},
	} {
		b, cleaned, ok, found := parseDelegationBlock(tc.in)
		if ok != tc.ok || found != tc.found || b.Target != tc.target || cleaned != tc.clean {
			t.Errorf("%s: ok=%v found=%v target=%q cleaned=%q", tc.name, ok, found, b.Target, cleaned)
		}
	}
}
