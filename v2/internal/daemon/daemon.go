// Package daemon keeps the device connected to the relay and runs incoming
// calls with the configured ACP agent in bypass mode.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/collabos/collabos/v2/internal/acp"
	"github.com/collabos/collabos/v2/internal/config"
	"github.com/collabos/collabos/v2/internal/gitctx"
	"github.com/collabos/collabos/v2/internal/wire"
)

// Runner executes one call and returns the answer; replaceable in tests.
type Runner func(ctx context.Context, c wire.Call, progress func(string)) (string, error)

// Daemon is the device agent host.
type Daemon struct {
	Cfg     config.Config
	Log     *slog.Logger
	Run     Runner
	Scratch string

	wmu    sync.Mutex
	ws     *websocket.Conn
	mu     sync.Mutex
	active map[string]context.CancelFunc
	slots  chan struct{}
}

// New builds a daemon that runs calls with the configured ACP agent.
func New(cfg config.Config, log *slog.Logger) *Daemon {
	n := cfg.MaxConcurrent
	if n <= 0 {
		n = 2
	}
	d := &Daemon{Cfg: cfg, Log: log, active: map[string]context.CancelFunc{}, slots: make(chan struct{}, n),
		Scratch: filepath.Join(config.Home(), "calls")}
	d.Run = d.runACP
	return d
}

// AgentLabel describes the configured agent (and its version when cheap).
func (d *Daemon) AgentLabel() string {
	argv := d.Cfg.AgentArgv()
	label := strings.Join(argv, " ")
	if argv[0] == "opencode" {
		if out, err := exec.Command("opencode", "--version").Output(); err == nil {
			label = "opencode " + strings.TrimSpace(string(out))
		}
	}
	return label
}

// Serve connects and reconnects until ctx ends.
func (d *Daemon) Serve(ctx context.Context) error {
	if d.Cfg.Server == "" || d.Cfg.Token == "" {
		return errors.New("not configured: run `collab login --server URL --token TOKEN`")
	}
	label := d.AgentLabel()
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := d.session(ctx, label)
		if ctx.Err() != nil {
			break
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		d.Log.Warn("disconnected; reconnecting", "err", err, "in", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	d.mu.Lock()
	for _, c := range d.active {
		c()
	}
	d.mu.Unlock()
	return nil
}

func wsURL(server string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/ws"
	return u.String(), nil
}

func (d *Daemon) session(ctx context.Context, label string) error {
	u, err := wsURL(d.Cfg.Server)
	if err != nil {
		return err
	}
	h := http.Header{"Authorization": {"Bearer " + d.Cfg.Token}}
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, u, h)
	if err != nil {
		if resp != nil && resp.StatusCode == 401 {
			return fmt.Errorf("server rejected the device token: %w", err)
		}
		return err
	}
	defer ws.Close()
	ws.SetReadLimit(8 << 20)
	host, _ := os.Hostname()
	d.mu.Lock()
	active := []string{}
	for id := range d.active {
		active = append(active, id)
	}
	d.mu.Unlock()
	d.wmu.Lock()
	d.ws = ws
	err = ws.WriteJSON(wire.Frame{Type: "hello", Agent: label, Hostname: host, OS: runtime.GOOS + "/" + runtime.GOARCH, Active: active})
	d.wmu.Unlock()
	if err != nil {
		return err
	}
	d.Log.Info("connected", "server", d.Cfg.Server, "agent", label)
	defer func() {
		d.wmu.Lock()
		d.ws = nil
		d.wmu.Unlock()
	}()
	stop := context.AfterFunc(ctx, func() { _ = ws.Close() })
	defer stop()
	ws.SetPingHandler(func(m string) error {
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		d.wmu.Lock()
		defer d.wmu.Unlock()
		return ws.WriteControl(websocket.PongMessage, []byte(m), time.Now().Add(10*time.Second))
	})
	for {
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		var f wire.Frame
		if err := ws.ReadJSON(&f); err != nil {
			return err
		}
		switch f.Type {
		case "call":
			if f.Call != nil {
				d.start(ctx, *f.Call)
			}
		case "cancel":
			d.mu.Lock()
			if c := d.active[f.ID]; c != nil {
				c()
			}
			d.mu.Unlock()
		}
	}
}

// send writes a frame, retrying across reconnects for results.
func (d *Daemon) send(ctx context.Context, f wire.Frame, retry bool) {
	for i := 0; ; i++ {
		d.wmu.Lock()
		ws := d.ws
		var err error
		if ws != nil {
			_ = ws.SetWriteDeadline(time.Now().Add(15 * time.Second))
			err = ws.WriteJSON(f)
		} else {
			err = errors.New("not connected")
		}
		d.wmu.Unlock()
		if err == nil || !retry || i > 120 || ctx.Err() != nil {
			return
		}
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func (d *Daemon) start(ctx context.Context, c wire.Call) {
	d.mu.Lock()
	if _, dup := d.active[c.ID]; dup {
		d.mu.Unlock()
		return
	}
	timeout := time.Duration(c.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	d.active[c.ID] = cancel
	d.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			d.mu.Lock()
			delete(d.active, c.ID)
			d.mu.Unlock()
		}()
		progress := func(s string) { d.send(ctx, wire.Frame{Type: "progress", ID: c.ID, Text: s}, false) }
		select {
		case d.slots <- struct{}{}:
		case <-cctx.Done():
			d.send(ctx, wire.Frame{Type: "result", ID: c.ID, Status: wire.StatusCanceled, Error: "canceled while queued"}, true)
			return
		}
		defer func() { <-d.slots }()
		progress("received by " + c.ToDevice)
		d.Log.Info("call started", "id", c.ID, "from", c.FromOwner+"/"+c.FromDevice)
		t0 := time.Now()
		text, err := d.Run(cctx, c, progress)
		f := wire.Frame{Type: "result", ID: c.ID, Status: wire.StatusSucceeded, Text: text}
		if err != nil {
			f.Status, f.Error = wire.StatusFailed, err.Error()
			if errors.Is(cctx.Err(), context.Canceled) {
				f.Status = wire.StatusCanceled
			}
			if errors.Is(cctx.Err(), context.DeadlineExceeded) {
				f.Error = "timed out after " + timeout.String()
			}
		}
		d.Log.Info("call finished", "id", c.ID, "status", f.Status, "took", time.Since(t0).Round(time.Second), "err", f.Error)
		d.send(ctx, f, true)
	}()
}

// BuildPrompt is what the callee's agent receives.
func BuildPrompt(c wire.Call, note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are receiving a request from %s's coding agent (device %s), relayed by CollabOS.\n", c.FromOwner, c.FromDevice)
	b.WriteString("Do the work yourself in this directory. Your final message is returned verbatim to the caller's agent, so end with a complete, self-contained answer.\n\n")
	b.WriteString("## Request\n\n")
	b.WriteString(c.Prompt)
	b.WriteString("\n")
	if cc := c.Context; cc != nil {
		b.WriteString("\n## Caller's code\n\n")
		if cc.Remote != "" {
			fmt.Fprintf(&b, "- repository: %s\n", cc.Remote)
		}
		if cc.Branch != "" {
			fmt.Fprintf(&b, "- branch: %s (head %s)\n", cc.Branch, cc.Head)
		}
		if cc.Base != "" {
			fmt.Fprintf(&b, "- changes are relative to commit %s\n", cc.Base)
		}
		if note != "" {
			fmt.Fprintf(&b, "- %s\n", note)
		}
		if cc.Note != "" {
			fmt.Fprintf(&b, "- note from caller: %s\n", cc.Note)
		}
		if cc.Patch != "" {
			p := cc.Patch
			if len(p) > 120<<10 {
				p = p[:120<<10] + "\n…[patch truncated here; full patch in ./CHANGES.patch]\n"
			}
			b.WriteString("\nThe full patch is in ./CHANGES.patch:\n\n```diff\n")
			b.WriteString(p)
			b.WriteString("```\n")
		} else {
			b.WriteString("- no uncommitted or branch changes were found\n")
		}
	}
	return b.String()
}

func (d *Daemon) runACP(ctx context.Context, c wire.Call, progress func(string)) (string, error) {
	_ = os.MkdirAll(d.Scratch, 0o755)
	repo := ""
	if c.Context != nil {
		repo = d.Cfg.RepoFor(c.Context.Remote)
	}
	dir, note, cleanup := gitctx.Workspace(ctx, d.Scratch, c.ID, repo, c.Context)
	defer cleanup()
	argv := d.Cfg.AgentArgv()
	env := []string{"COLLAB_CALL_ID=" + c.ID, "COLLAB_CALL_DEPTH=1"}
	for k, v := range d.Cfg.Env {
		env = append(env, k+"="+v)
	}
	if filepath.Base(strings.TrimSuffix(strings.ToLower(argv[0]), ".exe")) == "opencode" {
		oc := map[string]any{
			"$schema":    "https://opencode.ai/config.json",
			"permission": map[string]string{"edit": "allow", "bash": "allow", "webfetch": "allow", "external_directory": "allow"},
			"autoupdate": false,
			"share":      "disabled",
		}
		if d.Cfg.Model != "" {
			oc["model"] = d.Cfg.Model
		}
		b, _ := json.Marshal(oc)
		env = append(env, "OPENCODE_CONFIG_CONTENT="+string(b), "OPENCODE_DISABLE_AUTOUPDATE=1")
	}
	logf, _ := os.OpenFile(filepath.Join(config.Home(), "agent-stderr.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if logf != nil {
		defer logf.Close()
	}
	job := acp.Job{Argv: argv, Dir: dir, Env: env, Prompt: BuildPrompt(c, note), Progress: progress}
	if logf != nil {
		job.Stderr = logf
	}
	res, err := acp.Run(ctx, job)
	if err != nil {
		return res.Text, err
	}
	if res.Text == "" {
		return "", fmt.Errorf("agent finished (%s) without a reply", res.StopReason)
	}
	return res.Text, nil
}
