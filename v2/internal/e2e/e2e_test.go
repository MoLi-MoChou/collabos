package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/collabos/collabos/v2/internal/config"
	"github.com/collabos/collabos/v2/internal/daemon"
	"github.com/collabos/collabos/v2/internal/mcp"
	"github.com/collabos/collabos/v2/internal/server"
	"github.com/collabos/collabos/v2/internal/wire"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type env struct {
	srv    *httptest.Server
	tokens map[string]string
}

func setup(t *testing.T, devices ...[2]string) *env {
	st := server.NewMemStore()
	e := &env{tokens: map[string]string{}}
	for _, d := range devices {
		tok, h := server.NewToken()
		_ = st.AddDevice(context.Background(), d[0], d[1], h)
		e.tokens[d[0]] = tok
	}
	e.srv = httptest.NewServer(server.New(st, quiet()).Handler())
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) daemon(t *testing.T, name string, run daemon.Runner) {
	d := daemon.New(config.Config{Server: e.srv.URL, Token: e.tokens[name], Agent: []string{"true"}}, quiet())
	d.Run = run
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = d.Serve(ctx) }()
	api := e.api(name)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ds, _ := api.Agents(context.Background())
		for _, x := range ds {
			if x.Name == name && x.Online {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("daemon %s never came online", name)
}

func (e *env) api(name string) *mcp.API { return &mcp.API{Server: e.srv.URL, Token: e.tokens[name]} }

// mcpSession drives `collab mcp` over in-memory pipes.
func mcpSession(t *testing.T, api *mcp.API, cwd string, reqs ...string) []map[string]any {
	in := strings.Join(reqs, "\n") + "\n"
	var out bytes.Buffer
	s := &mcp.Server{API: api, Cwd: cwd, MaxWait: time.Minute}
	if err := s.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad line %q", l)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func resultText(t *testing.T, msgs []map[string]any, id float64) (string, bool) {
	for _, m := range msgs {
		if m["id"] == id {
			r := m["result"].(map[string]any)
			c := r["content"].([]any)[0].(map[string]any)
			return c["text"].(string), r["isError"].(bool)
		}
	}
	t.Fatalf("no reply for id %v in %v", id, msgs)
	return "", false
}

func gitRepo(t *testing.T) string {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("remote", "add", "origin", "https://github.com/acme/demo.git")
	writeFile(t, dir+"/a.py", "print('a')\n")
	run("add", ".")
	run("commit", "-qm", "init")
	return dir
}

func TestReviewRoundTripThroughMCP(t *testing.T) {
	e := setup(t, [2]string{"kai-pc", "kai"}, [2]string{"zs-box", "zhangsan"})
	got := make(chan wire.Call, 1)
	e.daemon(t, "zs-box", func(ctx context.Context, c wire.Call, progress func(string)) (string, error) {
		got <- c
		progress("reading patch")
		time.Sleep(100 * time.Millisecond)
		return "LGTM, but rename x to count", nil
	})
	e.daemon(t, "kai-pc", func(context.Context, wire.Call, func(string)) (string, error) { return "", nil })
	repo := gitRepo(t)
	writeFile(t, repo+"/a.py", "x = 1\nprint(x)\n")
	writeFile(t, repo+"/new.py", "print('new')\n")

	msgs := mcpSession(t, e.api("kai-pc"), repo,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"call_agent","arguments":{"target":"zhangsan","prompt":"review please"},"_meta":{"progressToken":"p1"}}}`,
	)
	for _, m := range msgs {
		if m["id"] == float64(2) {
			for _, tl := range m["result"].(map[string]any)["tools"].([]any) {
				if _, ok := tl.(map[string]any)["inputSchema"].(map[string]any)["required"].([]any); !ok {
					t.Fatalf("tool schema required must be an array: %v", tl)
				}
			}
		}
	}
	list, _ := resultText(t, msgs, 3)
	if !strings.Contains(list, "zs-box") || !strings.Contains(list, "online") {
		t.Fatalf("list_agents: %s", list)
	}
	text, isErr := resultText(t, msgs, 4)
	if isErr || !strings.Contains(text, "LGTM, but rename x to count") || !strings.Contains(text, "succeeded") {
		t.Fatalf("call_agent: %v %s", isErr, text)
	}
	c := <-got
	if c.FromOwner != "kai" || c.Context == nil || c.Context.Remote != "https://github.com/acme/demo.git" || c.Context.Branch != "main" {
		t.Fatalf("context: %+v", c.Context)
	}
	if !strings.Contains(c.Context.Patch, "+x = 1") || !strings.Contains(c.Context.Patch, "new.py") {
		t.Fatalf("patch misses changes:\n%s", c.Context.Patch)
	}
	progress := 0
	for _, m := range msgs {
		if m["method"] == "notifications/progress" {
			progress++
		}
	}
	if progress == 0 {
		t.Fatal("no progress notifications")
	}
	// recorded for history
	cs, err := e.api("kai-pc").List(context.Background(), 5)
	if err != nil || len(cs) != 1 || cs[0].Status != wire.StatusSucceeded {
		t.Fatalf("history: %v %+v", err, cs)
	}
	// index untouched
	out, _ := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if !strings.Contains(string(out), "?? new.py") {
		t.Fatalf("real index was modified: %s", out)
	}
}

func TestOfflineAndUnknownTargets(t *testing.T) {
	e := setup(t, [2]string{"kai-pc", "kai"}, [2]string{"zs-box", "zhangsan"})
	_, err := e.api("kai-pc").Create(context.Background(), wire.CallRequest{Target: "zhangsan", Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("want offline error, got %v", err)
	}
	_, err = e.api("kai-pc").Create(context.Background(), wire.CallRequest{Target: "nobody", Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "known: zhangsan/zs-box") {
		t.Fatalf("want unknown error, got %v", err)
	}
	if _, err := (&mcp.API{Server: e.srv.URL, Token: "cdv_bad"}).Agents(context.Background()); err == nil {
		t.Fatal("bad token accepted")
	}
}

func TestFailureAndTimeoutAndCancel(t *testing.T) {
	e := setup(t, [2]string{"kai-pc", "kai"}, [2]string{"zs-box", "zhangsan"})
	e.daemon(t, "zs-box", func(ctx context.Context, c wire.Call, _ func(string)) (string, error) {
		if c.Prompt == "boom" {
			return "", context.DeadlineExceeded
		}
		<-ctx.Done()
		return "", ctx.Err()
	})
	api := e.api("kai-pc")
	ctx := context.Background()
	c, err := api.Create(ctx, wire.CallRequest{Target: "zs-box", Prompt: "boom"})
	if err != nil {
		t.Fatal(err)
	}
	c, _ = api.Wait(ctx, c.ID, time.Now().Add(5*time.Second), nil)
	if c.Status != wire.StatusFailed {
		t.Fatalf("want failed, got %+v", c)
	}
	c, _ = api.Create(ctx, wire.CallRequest{Target: "zs-box", Prompt: "slow", TimeoutSec: 1})
	c, _ = api.Wait(ctx, c.ID, time.Now().Add(10*time.Second), nil)
	if c.Status != wire.StatusFailed || !strings.Contains(c.Error, "timed out") {
		t.Fatalf("want timeout, got %+v", c)
	}
	c, _ = api.Create(ctx, wire.CallRequest{Target: "zs-box", Prompt: "slow"})
	time.Sleep(100 * time.Millisecond)
	if _, err := api.Cancel(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	c, _ = api.Get(ctx, c.ID, 0, "")
	if c.Status != wire.StatusCanceled {
		t.Fatalf("want canceled, got %+v", c)
	}
	// get_call_result via MCP on a finished call
	msgs := mcpSession(t, api, t.TempDir(), `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"get_call_result","arguments":{"call_id":"`+c.ID+`","wait_seconds":0}}}`)
	text, _ := resultText(t, msgs, 9)
	if !strings.Contains(text, "canceled") {
		t.Fatal(text)
	}
}
