// Package acp runs one prompt against an ACP agent (opencode acp,
// claude-agent-acp, codex-acp, gemini --acp, ...) over stdio JSON-RPC, in
// bypass mode: every permission request is allowed.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Job is one run.
type Job struct {
	Argv   []string
	Dir    string
	Env    []string // appended to the daemon environment
	Prompt string
	// Progress receives short human-readable activity lines.
	Progress func(string)
	// Stderr, when set, receives the agent's stderr.
	Stderr io.Writer
	// Inactivity fails the run when the agent sends nothing for this long
	// (0 disables).
	Inactivity time.Duration
}

// ErrInactive is returned when the agent went silent for Job.Inactivity.
var ErrInactive = errors.New("agent inactive")

// Result is the agent's final answer.
type Result struct {
	Text       string
	StopReason string
	Agent      string
}

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type client struct {
	w       io.Writer
	wmu     sync.Mutex
	next    atomic.Int64
	pmu     sync.Mutex
	pending map[string]chan rpcMsg
}

func (c *client) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.w.Write(append(b, '\n'))
	return err
}

func (c *client) call(ctx context.Context, method string, params any, out any) error {
	id := c.next.Add(1)
	key := fmt.Sprint(id)
	ch := make(chan rpcMsg, 1)
	c.pmu.Lock()
	c.pending[key] = ch
	c.pmu.Unlock()
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return fmt.Errorf("%s: %s (code %d)", method, m.Error.Message, m.Error.Code)
		}
		if out != nil {
			return json.Unmarshal(m.Result, out)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PickAllow chooses the most permissive allow option.
func PickAllow(options []struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
}) string {
	for _, want := range []string{"allow_always", "allow_once"} {
		for _, o := range options {
			if o.Kind == want {
				return o.OptionID
			}
		}
	}
	if len(options) > 0 {
		return options[0].OptionID
	}
	return ""
}

// Run starts the agent, sends one prompt and returns the reply text.
func Run(ctx context.Context, j Job) (Result, error) {
	if len(j.Argv) == 0 {
		return Result{}, errors.New("no agent command configured")
	}
	cmd := exec.CommandContext(ctx, j.Argv[0], j.Argv[1:]...)
	cmd.Dir = j.Dir
	cmd.Env = append(os.Environ(), j.Env...)
	setProcAttr(cmd)
	cmd.Cancel = func() error { return killTree(cmd) }
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if j.Stderr != nil {
		cmd.Stderr = j.Stderr
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", j.Argv[0], err)
	}
	defer func() {
		_ = stdin.Close()
		_ = killTree(cmd)
		_ = cmd.Wait()
	}()
	progress := j.Progress
	if progress == nil {
		progress = func(string) {}
	}
	c := &client{w: stdin, pending: map[string]chan rpcMsg{}}
	var (
		tmu      sync.Mutex
		text     strings.Builder
		all      strings.Builder
		lastKind string
	)
	dead := make(chan struct{})
	var lastActive atomic.Int64
	lastActive.Store(time.Now().UnixNano())
	go func() {
		defer close(dead)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			lastActive.Store(time.Now().UnixNano())
			var m rpcMsg
			if json.Unmarshal(sc.Bytes(), &m) != nil {
				continue
			}
			switch {
			case m.Method != "" && len(m.ID) > 0: // request from the agent
				go c.answer(m)
			case m.Method == "session/update":
				var p struct {
					Update struct {
						Kind    string `json:"sessionUpdate"`
						Content struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"content"`
						Title  string `json:"title"`
						Status string `json:"status"`
					} `json:"update"`
				}
				if json.Unmarshal(m.Params, &p) != nil {
					continue
				}
				u := p.Update
				switch u.Kind {
				case "agent_message_chunk":
					tmu.Lock()
					if lastKind == "tool" && text.Len() > 0 {
						// A new message after tool use: the final answer is what follows.
						text.Reset()
					}
					text.WriteString(u.Content.Text)
					all.WriteString(u.Content.Text)
					lastKind = "text"
					tmu.Unlock()
				case "tool_call":
					tmu.Lock()
					lastKind = "tool"
					tmu.Unlock()
					if u.Title != "" {
						progress("tool: " + u.Title)
					}
				case "plan":
					progress("planning")
				}
			case len(m.ID) > 0:
				c.pmu.Lock()
				ch := c.pending[string(m.ID)]
				delete(c.pending, string(m.ID))
				c.pmu.Unlock()
				if ch != nil {
					ch <- m
				}
			}
		}
	}()
	rctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go func() {
		select {
		case <-dead:
			cancel(nil)
		case <-rctx.Done():
		}
	}()
	if j.Inactivity > 0 {
		go func() {
			t := time.NewTicker(min(j.Inactivity/4, 5*time.Second))
			defer t.Stop()
			for {
				select {
				case <-rctx.Done():
					return
				case <-t.C:
					if idle := time.Since(time.Unix(0, lastActive.Load())); idle >= j.Inactivity {
						cancel(fmt.Errorf("%w: no ACP activity for %s (model stuck or rate-limited?)", ErrInactive, idle.Round(time.Second)))
						return
					}
				}
			}
		}()
	}
	var init struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	if err := c.call(rctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{"fs": map[string]bool{"readTextFile": false, "writeTextFile": false}, "terminal": false},
		"clientInfo":         map[string]string{"name": "collab", "version": "2"},
	}, &init); err != nil {
		return Result{}, wrapDead(ctx, rctx, dead, err)
	}
	res := Result{Agent: strings.TrimSpace(init.AgentInfo.Name + " " + init.AgentInfo.Version)}
	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.call(rctx, "session/new", map[string]any{"cwd": j.Dir, "mcpServers": []any{}}, &sess); err != nil {
		return res, wrapDead(ctx, rctx, dead, err)
	}
	progress("agent started (" + res.Agent + ")")
	var pr struct {
		StopReason string `json:"stopReason"`
	}
	err = c.call(rctx, "session/prompt", map[string]any{
		"sessionId": sess.SessionID,
		"prompt":    []map[string]string{{"type": "text", "text": j.Prompt}},
	}, &pr)
	if err != nil {
		if rctx.Err() != nil {
			_ = c.write(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]string{"sessionId": sess.SessionID}})
		}
		return res, wrapDead(ctx, rctx, dead, err)
	}
	tmu.Lock()
	res.Text = strings.TrimSpace(text.String())
	if res.Text == "" {
		res.Text = strings.TrimSpace(all.String())
	}
	tmu.Unlock()
	res.StopReason = pr.StopReason
	return res, nil
}

func wrapDead(ctx, rctx context.Context, dead chan struct{}, err error) error {
	if c := context.Cause(rctx); c != nil && errors.Is(c, ErrInactive) {
		return c
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case <-dead:
		return fmt.Errorf("agent exited: %w", err)
	default:
		return err
	}
}

func (c *client) answer(m rpcMsg) {
	reply := func(result any) {
		_ = c.write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
	}
	switch m.Method {
	case "session/request_permission":
		var p struct {
			Options []struct {
				OptionID string `json:"optionId"`
				Kind     string `json:"kind"`
			} `json:"options"`
		}
		_ = json.Unmarshal(m.Params, &p)
		if id := PickAllow(p.Options); id != "" {
			reply(map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": id}})
			return
		}
		reply(map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
	default:
		_ = c.write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "method not supported: " + m.Method}})
	}
}
