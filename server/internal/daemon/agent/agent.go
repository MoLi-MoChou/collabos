// Package agent is the long-running daemon: it keeps one outbound WebSocket
// to the server, heartbeats, reports capabilities, executes dispatched tasks
// through the runner, relays permission requests, performs verified merges
// for its owner and journals every outgoing report for replay.
package agent

import (
	"context"
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

	"github.com/collabos/collabos/server/internal/daemon/config"
	"github.com/collabos/collabos/server/internal/daemon/journal"
	"github.com/collabos/collabos/server/internal/daemon/runner"
	"github.com/collabos/collabos/server/internal/daemon/sandbox"
	"github.com/collabos/collabos/server/internal/daemon/worktree"
	"github.com/collabos/collabos/server/internal/logger"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Version is the daemon version reported in capabilities.
const Version = "0.1.0"

// Daemon is the running agent host.
type Daemon struct {
	cfg     config.Config
	log     *slog.Logger
	journal *journal.Journal
	sandbox sandbox.Backend
	ocBin   string

	connMu sync.Mutex
	conn   *websocket.Conn
	ready  bool
	// sendMu orders journal appends with sends and replay, so the server
	// always sees seqs in increasing order.
	sendMu sync.Mutex

	tasksMu sync.Mutex
	tasks   map[string]context.CancelFunc

	waitMu  sync.Mutex
	waiters map[string]chan protocol.PermissionDecision
}

// New opens the journal and detects the sandbox.
func New(cfg config.Config) (*Daemon, error) {
	if cfg.ServerURL == "" || cfg.NodeToken == "" {
		return nil, errors.New("not logged in: run `collab login --server URL --token NODE_TOKEN`")
	}
	if err := os.MkdirAll(config.Home(), 0o700); err != nil {
		return nil, err
	}
	j, err := journal.Open(filepath.Join(config.Home(), "journal.db"))
	if err != nil {
		return nil, err
	}
	bin := cfg.OpencodeBin
	if bin == "" {
		bin = findOpencode()
	}
	return &Daemon{
		cfg: cfg, log: logger.New("daemon"), journal: j, sandbox: sandbox.Detect(), ocBin: bin,
		tasks: map[string]context.CancelFunc{}, waiters: map[string]chan protocol.PermissionDecision{},
	}, nil
}

func findOpencode() string {
	if p, err := exec.LookPath("opencode"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".opencode", "bin", "opencode")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func toolVersion(bin string, args ...string) string {
	if bin == "" {
		return ""
	}
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// Capabilities reports what this host offers.
func (d *Daemon) Capabilities() protocol.Capabilities {
	host, _ := os.Hostname()
	tools := map[string]string{}
	if v := toolVersion("git", "--version"); v != "" {
		tools["git"] = v
	}
	if v := toolVersion(d.ocBin, "--version"); v != "" {
		tools["opencode"] = v
	}
	if p, err := exec.LookPath("bwrap"); err == nil {
		tools["bwrap"] = toolVersion(p, "--version")
	}
	if p, err := exec.LookPath("gh"); err == nil {
		tools["gh"] = toolVersion(p, "--version")
	}
	agents := []string{}
	if tools["opencode"] != "" {
		agents = append(agents, "opencode")
	}
	repos := []protocol.RepoInfo{}
	for _, r := range d.cfg.Repos {
		repos = append(repos, protocol.RepoInfo{RemoteURL: r.RemoteURL, Path: r.Path})
	}
	return protocol.Capabilities{OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: host, Tools: tools, Agents: agents, Sandbox: d.sandbox.Name(), Repos: repos, Version: Version}
}

// Run connects and reconnects until ctx ends.
func (d *Daemon) Run(ctx context.Context) error {
	backoff := time.Second
	for ctx.Err() == nil {
		err := d.session(ctx)
		if ctx.Err() != nil {
			break
		}
		d.log.Warn("disconnected; reconnecting", "err", err, "in", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
		}
		if backoff < 15*time.Second {
			backoff *= 2
		} else {
			backoff = 15 * time.Second
		}
	}
	d.cancelAll()
	return d.journal.Close()
}

func wsURL(server string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/daemon/ws"
	return u.String(), nil
}

func (d *Daemon) session(ctx context.Context) error {
	u, err := wsURL(d.cfg.ServerURL)
	if err != nil {
		return err
	}
	hdr := http.Header{"Authorization": {"Bearer " + d.cfg.NodeToken}}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u, hdr)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("server rejected node token (401): re-pair this device")
		}
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(8 << 20)
	journaled, _ := d.journal.Tasks()
	caps := d.Capabilities()
	hs := protocol.ClientHandshake{ClientType: protocol.ClientDaemon, Capabilities: &caps, ActiveTasks: d.activeTasks(), JournaledTasks: journaled}
	if err := conn.WriteJSON(protocol.Msg(protocol.TypeClientHandshake, hs)); err != nil {
		return err
	}
	var ack protocol.Envelope
	if err := conn.ReadJSON(&ack); err != nil {
		return err
	}
	var hsAck protocol.ServerHandshakeAck
	if ack.Type != protocol.TypeServerHandshakeAck || ack.Into(&hsAck) != nil {
		return errors.New("bad handshake ack")
	}
	if err := d.journal.Sync(hsAck.LastAckedSeq); err != nil {
		return err
	}
	d.connMu.Lock()
	d.conn = conn
	d.connMu.Unlock()
	d.log.Info("connected", "daemon_id", hsAck.DaemonID, "sandbox", caps.Sandbox, "last_acked_seq", hsAck.LastAckedSeq)
	// Replay unacknowledged frames in order before anything new.
	d.sendMu.Lock()
	pending, _ := d.journal.Pending()
	for _, e := range pending {
		if err := d.write(e); err != nil {
			d.sendMu.Unlock()
			return err
		}
	}
	if len(pending) > 0 {
		d.log.Info("replayed journal", "frames", len(pending))
	}
	d.connMu.Lock()
	d.ready = true
	d.connMu.Unlock()
	d.sendMu.Unlock()
	defer func() {
		d.connMu.Lock()
		d.conn, d.ready = nil, false
		d.connMu.Unlock()
	}()

	hb := time.Duration(hsAck.HeartbeatInterval) * time.Second
	if hb <= 0 {
		hb = 15 * time.Second
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		t := time.NewTicker(hb)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				_ = conn.Close()
				return
			case <-t.C:
				if err := d.write(protocol.Msg(protocol.TypeHeartbeatPing, map[string]any{})); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(3*hb + 10*time.Second))
		var e protocol.Envelope
		if err := conn.ReadJSON(&e); err != nil {
			return err
		}
		d.handle(ctx, e)
	}
}

func (d *Daemon) write(e protocol.Envelope) error {
	d.connMu.Lock()
	defer d.connMu.Unlock()
	if d.conn == nil {
		return errors.New("not connected")
	}
	_ = d.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return d.conn.WriteJSON(e)
}

// report journals a frame and sends it if connected; it is replayed otherwise.
func (d *Daemon) report(taskID, typ string, payload any) {
	d.sendMu.Lock()
	defer d.sendMu.Unlock()
	e, err := d.journal.Append(taskID, protocol.Msg(typ, payload))
	if err != nil {
		d.log.Error("journal append", "err", err)
		return
	}
	d.connMu.Lock()
	ready := d.ready
	d.connMu.Unlock()
	if ready {
		_ = d.write(e)
	}
}

func (d *Daemon) handle(ctx context.Context, e protocol.Envelope) {
	switch e.Type {
	case protocol.TypeHeartbeatPong:
	case protocol.TypeAck:
		var a protocol.Ack
		if e.Into(&a) == nil {
			_ = d.journal.Ack(a.AckSeq)
		}
	case protocol.TypeDelegationDispatch:
		var t protocol.DelegationDispatch
		if e.Into(&t) == nil {
			d.startTask(ctx, t)
		}
	case protocol.TypeTaskCancel:
		var c protocol.TaskCancel
		if e.Into(&c) == nil {
			d.tasksMu.Lock()
			if cancel := d.tasks[c.TaskID]; cancel != nil {
				cancel()
			}
			d.tasksMu.Unlock()
		}
	case protocol.TypePermissionDecision:
		var p protocol.PermissionDecision
		if e.Into(&p) == nil {
			d.waitMu.Lock()
			ch := d.waiters[p.TaskID+"/"+p.RequestID]
			delete(d.waiters, p.TaskID+"/"+p.RequestID)
			d.waitMu.Unlock()
			if ch != nil {
				ch <- p
			}
		}
	case protocol.TypeMergeExecute:
		var m protocol.MergeExecute
		if e.Into(&m) == nil {
			go d.merge(ctx, m)
		}
	default:
		d.log.Debug("ignored frame", "type", e.Type)
	}
}

func (d *Daemon) activeTasks() []string {
	d.tasksMu.Lock()
	defer d.tasksMu.Unlock()
	out := []string{}
	for id := range d.tasks {
		out = append(out, id)
	}
	return out
}

func (d *Daemon) cancelAll() {
	d.tasksMu.Lock()
	defer d.tasksMu.Unlock()
	for _, c := range d.tasks {
		c()
	}
}

func (d *Daemon) startTask(ctx context.Context, t protocol.DelegationDispatch) {
	d.tasksMu.Lock()
	if _, busy := d.tasks[t.TaskID]; busy {
		d.tasksMu.Unlock()
		return
	}
	tctx, cancel := context.WithCancel(ctx)
	d.tasks[t.TaskID] = cancel
	d.tasksMu.Unlock()
	go func() {
		defer func() {
			d.tasksMu.Lock()
			delete(d.tasks, t.TaskID)
			d.tasksMu.Unlock()
			cancel()
		}()
		d.log.Info("task started", "task", t.TaskID, "kind", t.Kind, "title", t.Title)
		repo := ""
		if t.Kind == protocol.KindCode {
			repo, _ = d.cfg.RepoFor(t.RemoteURL)
		}
		if d.ocBin == "" {
			d.report(t.TaskID, protocol.TypeDaemonTaskCompleted, protocol.DaemonTaskCompleted{TaskID: t.TaskID, Outcome: protocol.OutcomeFailed, ReasonCode: "agent_missing", Error: "opencode not installed on this device"})
			return
		}
		scratch := filepath.Join(config.Home(), "scratch")
		_ = os.MkdirAll(scratch, 0o700)
		res := runner.Run(tctx, runner.Job{
			Task: t, RepoPath: repo, Sandbox: d.sandbox, Opencode: d.ocBin, WorkDir: scratch,
			Hooks: runner.Hooks{
				Progress: func(p protocol.DaemonProgress) { d.report(t.TaskID, protocol.TypeDaemonProgressStream, p) },
				Blocked:  func(p protocol.PolicyBlocked) { d.report(t.TaskID, protocol.TypeDaemonPolicyBlocked, p) },
				Escalate: d.escalate,
			},
		})
		d.log.Info("task finished", "task", t.TaskID, "outcome", res.Outcome, "commit", res.Commit, "err", res.Error)
		d.report(t.TaskID, protocol.TypeDaemonTaskCompleted, res)
	}()
}

func (d *Daemon) escalate(ctx context.Context, req protocol.PermissionRequest) (protocol.PermissionDecision, error) {
	key := req.TaskID + "/" + req.RequestID
	ch := make(chan protocol.PermissionDecision, 1)
	d.waitMu.Lock()
	d.waiters[key] = ch
	d.waitMu.Unlock()
	d.report(req.TaskID, protocol.TypePermissionRequest, req)
	select {
	case dec := <-ch:
		return dec, nil
	case <-ctx.Done():
		d.waitMu.Lock()
		delete(d.waiters, key)
		d.waitMu.Unlock()
		return protocol.PermissionDecision{}, ctx.Err()
	}
}

// merge performs an approved merge with this device owner's own credentials.
func (d *Daemon) merge(ctx context.Context, m protocol.MergeExecute) {
	res := protocol.MergeResult{ApprovalID: m.ApprovalID, TaskID: m.TaskID}
	repo, ok := d.cfg.RepoFor(m.RemoteURL)
	if !ok {
		res.Error = "repository not configured on approver device"
		d.report("", protocol.TypeMergeResult, res)
		return
	}
	mctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if m.Provider == "github" {
		res = githubMerge(mctx, repo, m)
		d.report("", protocol.TypeMergeResult, res)
		return
	}
	out, err := worktree.VerifiedMerge(mctx, repo, m.Branch, m.BaseBranch, m.ApprovedCommit, m.Message)
	res.HeadCommit, res.MergeCommit = out.HeadCommit, out.MergeCommit
	if err != nil {
		res.Error = err.Error()
	} else {
		res.OK = true
	}
	d.log.Info("merge", "task", m.TaskID, "ok", res.OK, "merge_commit", res.MergeCommit, "err", res.Error)
	d.report("", protocol.TypeMergeResult, res)
}

// githubMerge merges with the approver's own gh login, pinned to the approved
// head commit. Not runtime-verified here (no GitHub test repository in CI).
func githubMerge(ctx context.Context, repo string, m protocol.MergeExecute) protocol.MergeResult {
	res := protocol.MergeResult{ApprovalID: m.ApprovalID, TaskID: m.TaskID}
	head, err := worktree.Git(ctx, repo, "ls-remote", "origin", "refs/heads/"+m.Branch)
	if err == nil {
		res.HeadCommit = strings.Fields(head + " ")[0]
	}
	ref := m.PRURL
	if ref == "" {
		ref = fmt.Sprint(m.PRNumber)
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "merge", ref, "--merge", "--match-head-commit", m.ApprovedCommit)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		res.Error = fmt.Sprintf("gh pr merge: %v: %s", err, out)
		return res
	}
	if _, err := worktree.Git(ctx, repo, "fetch", "origin", m.BaseBranch); err == nil {
		res.MergeCommit, _ = worktree.Git(ctx, repo, "rev-parse", "origin/"+m.BaseBranch)
	}
	res.OK = true
	return res
}
