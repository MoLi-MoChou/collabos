package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PermissionOption is one choice offered by the agent.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

// PermissionToolCall describes what the agent wants to do.
type PermissionToolCall struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Locations  []struct {
		Path string `json:"path"`
	} `json:"locations"`
	RawInput json.RawMessage `json:"rawInput"`
}

// PermissionRequest is session/request_permission params.
type PermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  PermissionToolCall `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// Command extracts a shell command from rawInput when present.
func (p PermissionRequest) Command() string {
	var in struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(p.ToolCall.RawInput, &in)
	return in.Command
}

// Paths lists the locations the tool call touches.
func (p PermissionRequest) Paths() []string {
	out := make([]string, 0, len(p.ToolCall.Locations))
	for _, l := range p.ToolCall.Locations {
		out = append(out, l.Path)
	}
	return out
}

// Decider decides a permission request; true = allow once.
type Decider func(ctx context.Context, req PermissionRequest) (bool, error)

// Options configures a Client.
type Options struct {
	// Root confines client-side fs/* calls (PEP layer 1).
	Root     string
	OnUpdate func(sessionID string, update json.RawMessage)
	Decide   Decider
	// CheckWrite may veto an fs/write_text_file after confinement (grant check).
	CheckWrite func(path string) error
	Trace      func(dir string, raw []byte)
}

// InitializeResult is the subset of the agent's initialize answer we keep.
type InitializeResult struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities"`
	AgentInfo         struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agentInfo"`
}

// PromptResult is session/prompt's result.
type PromptResult struct {
	StopReason string `json:"stopReason"`
	Usage      *struct {
		InputTokens      int64 `json:"inputTokens"`
		OutputTokens     int64 `json:"outputTokens"`
		ThoughtTokens    int64 `json:"thoughtTokens"`
		CachedReadTokens int64 `json:"cachedReadTokens"`
		TotalTokens      int64 `json:"totalTokens"`
	} `json:"usage"`
}

// Client drives one agent process.
type Client struct {
	conn    *Conn
	opts    Options
	adapter Adapter
	Init    InitializeResult
}

// ErrPathOutsideRoot is returned for fs access outside Options.Root.
var ErrPathOutsideRoot = errors.New("acp: path outside sandbox root")

// New wires a client to the agent's stdout (r) and stdin (w).
func New(r io.Reader, w io.Writer, opts Options) *Client {
	c := &Client{opts: opts}
	c.conn = NewConn(r, w, c.handleRequest, c.handleNotification)
	c.conn.Trace = opts.Trace
	return c
}

// Adapter returns the negotiated adapter (nil before Initialize).
func (c *Client) Adapter() Adapter { return c.adapter }

// Done is closed when the agent's stdout closes.
func (c *Client) Done() <-chan struct{} { return c.conn.Done() }

// Initialize offers PreferredVersion and negotiates down if needed.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": PreferredVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": true, "writeTextFile": true},
			"terminal": false,
		},
		"clientInfo": map[string]string{"name": "collabos-daemon", "version": "0.1.0"},
	}
	if err := c.conn.Call(ctx, "initialize", params, &c.Init); err != nil {
		return err
	}
	a, err := Negotiate(c.Init.ProtocolVersion)
	if err != nil {
		return err
	}
	c.adapter = a
	return nil
}

// NewSession starts a session rooted at cwd.
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	var res struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.conn.Call(ctx, "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}, &res); err != nil {
		return "", err
	}
	if res.SessionID == "" {
		return "", errors.New("acp: empty sessionId")
	}
	return res.SessionID, nil
}

// Prompt sends a text prompt and blocks until the turn ends.
func (c *Client) Prompt(ctx context.Context, sessionID, text string) (PromptResult, error) {
	var res PromptResult
	err := c.conn.Call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": text}},
	}, &res)
	return res, err
}

// Cancel asks the agent to stop the current turn.
func (c *Client) Cancel(sessionID string) error {
	return c.conn.Notify("session/cancel", map[string]string{"sessionId": sessionID})
}

func (c *Client) handleNotification(method string, params json.RawMessage) {
	if method != "session/update" || c.opts.OnUpdate == nil {
		return
	}
	var p struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	if json.Unmarshal(params, &p) == nil {
		c.opts.OnUpdate(p.SessionID, p.Update)
	}
}

func (c *Client) handleRequest(ctx context.Context, method string, params json.RawMessage) (any, *RPCError) {
	switch method {
	case "session/request_permission":
		var req PermissionRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		return c.decidePermission(ctx, req), nil
	case "fs/read_text_file":
		var p struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		path, err := c.confine(p.Path)
		if err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, &RPCError{Code: CodeInternal, Message: err.Error()}
		}
		return map[string]string{"content": string(b)}, nil
	case "fs/write_text_file":
		var p struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		path, err := c.confine(p.Path)
		if err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		if c.opts.CheckWrite != nil {
			if err := c.opts.CheckWrite(path); err != nil {
				return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, &RPCError{Code: CodeInternal, Message: err.Error()}
		}
		if err := os.WriteFile(path, []byte(p.Content), 0o644); err != nil {
			return nil, &RPCError{Code: CodeInternal, Message: err.Error()}
		}
		return map[string]any{}, nil
	}
	return nil, &RPCError{Code: CodeMethodNotFound, Message: "method not supported: " + method}
}

func (c *Client) decidePermission(ctx context.Context, req PermissionRequest) any {
	a := c.adapter
	if a == nil {
		a = v1Adapter{}
	}
	allow := false
	if c.opts.Decide != nil {
		ok, err := c.opts.Decide(ctx, req)
		allow = ok && err == nil
	}
	want := "reject_once"
	if allow {
		want = "allow_once"
	}
	for _, o := range req.Options {
		if o.Kind == want {
			return a.PermissionSelected(o.OptionID)
		}
	}
	return a.PermissionCancelled()
}

// confine resolves p and ensures it stays inside Root (symlinks resolved).
func (c *Client) confine(p string) (string, error) {
	return ConfinePath(c.opts.Root, p)
}

// ConfinePath returns the cleaned absolute path if it lies within root.
func ConfinePath(root, p string) (string, error) {
	if root == "" {
		return "", ErrPathOutsideRoot
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		// On Windows "/etc/x", "\\x" and "C:x" are not absolute but still name
		// a location outside root (current drive or drive-relative): refuse.
		if filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
			return "", ErrPathOutsideRoot
		}
		p = filepath.Join(rootReal, p)
	}
	clean := filepath.Clean(p)
	// Resolve the deepest existing ancestor so new files can be created.
	existing := clean
	var rest []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		rest = append([]string{filepath.Base(existing)}, rest...)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	full := filepath.Join(append([]string{real}, rest...)...)
	rel, err := filepath.Rel(rootReal, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathOutsideRoot
	}
	return full, nil
}
