package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeAgent answers initialize with version, then emits a permission request
// and an update during session/prompt.
func fakeAgent(t *testing.T, version int, in io.Reader, out io.Writer) {
	t.Helper()
	sc := bufio.NewScanner(in)
	enc := func(v any) { b, _ := json.Marshal(v); _, _ = out.Write(append(b, '\n')) }
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		id := m["id"]
		switch m["method"] {
		case "initialize":
			enc(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": version, "agentInfo": map[string]string{"name": "fake"}}})
		case "session/new":
			enc(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"sessionId": "s1"}})
		case "session/prompt":
			enc(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": map[string]any{"sessionUpdate": "agent_message_chunk"}}})
			enc(map[string]any{"jsonrpc": "2.0", "id": 900, "method": "session/request_permission", "params": map[string]any{
				"sessionId": "s1", "toolCall": map[string]any{"kind": "execute", "title": "ls", "rawInput": map[string]string{"command": "ls"}},
				"options": []map[string]string{{"optionId": "once", "kind": "allow_once"}, {"optionId": "reject", "kind": "reject_once"}},
			}})
			// Wait for the permission answer before ending the turn.
			if sc.Scan() {
				var ans map[string]any
				_ = json.Unmarshal(sc.Bytes(), &ans)
				enc(map[string]any{"jsonrpc": "2.0", "method": "test/permission_answer", "params": ans["result"]})
			}
			enc(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"stopReason": "end_turn"}})
		}
	}
}

func run(t *testing.T, version int, allow bool) (*Client, []string, error) {
	ar, cw := io.Pipe()
	cr, aw := io.Pipe()
	go fakeAgent(t, version, ar, aw)
	var updates []string
	c := New(cr, cw, Options{
		Root:     t.TempDir(),
		OnUpdate: func(_ string, u json.RawMessage) { updates = append(updates, string(u)) },
		Decide:   func(context.Context, PermissionRequest) (bool, error) { return allow, nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Initialize(ctx); err != nil {
		return c, nil, err
	}
	sid, err := c.NewSession(ctx, "/tmp")
	if err != nil {
		return c, nil, err
	}
	res, err := c.Prompt(ctx, sid, "hi")
	if err == nil && res.StopReason != "end_turn" {
		t.Fatalf("stop reason %q", res.StopReason)
	}
	return c, updates, err
}

func TestNegotiateV1Fallback(t *testing.T) {
	c, updates, err := run(t, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Adapter().Version() != 1 || c.Adapter().ResumeMethod() != "session/load" {
		t.Fatalf("expected v1 adapter, got %d", c.Adapter().Version())
	}
	if len(updates) != 1 {
		t.Fatalf("updates: %v", updates)
	}
}

func TestNegotiateV2(t *testing.T) {
	c, _, err := run(t, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Adapter().Version() != 2 || c.Adapter().AuthMethod() != "auth/login" {
		t.Fatalf("expected v2 adapter")
	}
}

func TestNegotiateRejectsUnknown(t *testing.T) {
	if _, err := Negotiate(3); err == nil {
		t.Fatal("v3 must be rejected")
	}
	if _, err := Negotiate(0); err == nil {
		t.Fatal("v0 must be rejected")
	}
}

func TestPermissionMapping(t *testing.T) {
	c := &Client{adapter: v1Adapter{}, opts: Options{Decide: func(context.Context, PermissionRequest) (bool, error) { return false, nil }}}
	req := PermissionRequest{Options: []PermissionOption{{OptionID: "a", Kind: "allow_once"}, {OptionID: "r", Kind: "reject_once"}}}
	got, _ := json.Marshal(c.decidePermission(context.Background(), req))
	if string(got) != `{"outcome":{"optionId":"r","outcome":"selected"}}` {
		t.Fatalf("deny mapping: %s", got)
	}
	c.opts.Decide = func(context.Context, PermissionRequest) (bool, error) { return true, nil }
	got, _ = json.Marshal(c.decidePermission(context.Background(), req))
	if string(got) != `{"outcome":{"optionId":"a","outcome":"selected"}}` {
		t.Fatalf("allow mapping: %s", got)
	}
	got, _ = json.Marshal(c.decidePermission(context.Background(), PermissionRequest{}))
	if string(got) != `{"outcome":{"outcome":"cancelled"}}` {
		t.Fatalf("no options: %s", got)
	}
}

func TestConfinePath(t *testing.T) {
	root := t.TempDir()
	if _, err := ConfinePath(root, filepath.Join(root, "a/b.txt")); err != nil {
		t.Fatalf("inside: %v", err)
	}
	if _, err := ConfinePath(root, "rel.txt"); err != nil {
		t.Fatalf("relative: %v", err)
	}
	for _, p := range []string{"/etc/passwd", filepath.Join(root, "../x"), "../../x"} {
		if _, err := ConfinePath(root, p); err == nil {
			t.Errorf("%s should be outside", p)
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{`\Windows\win.ini`, `C:x.txt`, `\\srv\share\f`} {
			if _, err := ConfinePath(root, p); err == nil {
				t.Errorf("%s should be outside", p)
			}
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfinePath(root, filepath.Join(root, "link/f")); err == nil {
		t.Error("symlink escape must be rejected")
	}
}
