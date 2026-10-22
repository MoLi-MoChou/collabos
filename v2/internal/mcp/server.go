// Package mcp is `collab mcp`: a stdio MCP server the user adds to their own
// agent (opencode, Claude Code, Codex, Gemini CLI, ...). It lets that agent
// list the other devices and call their agents, blocking until the answer
// comes back.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/collabos/collabos/v2/internal/gitctx"
	"github.com/collabos/collabos/v2/internal/wire"
)

// Server serves MCP over stdio.
type Server struct {
	API *API
	// MaxWait caps how long call_agent blocks before handing back a call id.
	MaxWait time.Duration
	// Cwd is the default directory for code context.
	Cwd string

	wmu     sync.Mutex
	out     io.Writer
	cmu     sync.Mutex
	cancels map[string]context.CancelFunc
}

type req struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (s *Server) write(v any) {
	b, _ := json.Marshal(v)
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, _ = s.out.Write(append(b, '\n'))
}

func (s *Server) reply(id json.RawMessage, result any) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) fail(id json.RawMessage, code int, msg string) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

// Serve reads requests from in until EOF.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out, s.cancels = out, map[string]context.CancelFunc{}
	if s.MaxWait <= 0 {
		s.MaxWait = 30 * time.Minute
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r req
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			s.fail(json.RawMessage("null"), -32700, "parse error")
			continue
		}
		switch r.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(r.Params, &p)
			v := p.ProtocolVersion
			if v == "" {
				v = "2025-06-18"
			}
			s.reply(r.ID, map[string]any{
				"protocolVersion": v,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "collab", "version": "2.0.0"},
				"instructions":    "Use these tools to delegate work to coding agents on other people's machines (e.g. \"ask zhangsan to review\"). call_agent blocks until the remote agent replies.",
			})
		case "notifications/initialized", "notifications/roots/list_changed":
		case "notifications/cancelled":
			var p struct {
				RequestID json.RawMessage `json:"requestId"`
			}
			_ = json.Unmarshal(r.Params, &p)
			s.cmu.Lock()
			if c := s.cancels[string(p.RequestID)]; c != nil {
				c()
			}
			s.cmu.Unlock()
		case "ping":
			s.reply(r.ID, map[string]any{})
		case "tools/list":
			s.reply(r.ID, map[string]any{"tools": tools()})
		case "tools/call":
			cctx, cancel := context.WithCancel(ctx)
			s.cmu.Lock()
			s.cancels[string(r.ID)] = cancel
			s.cmu.Unlock()
			wg.Add(1)
			go func(r req) {
				defer wg.Done()
				defer func() {
					cancel()
					s.cmu.Lock()
					delete(s.cancels, string(r.ID))
					s.cmu.Unlock()
				}()
				s.toolCall(cctx, r)
			}(r)
		default:
			if len(r.ID) > 0 {
				s.fail(r.ID, -32601, "method not found: "+r.Method)
			}
		}
	}
	wg.Wait()
	return sc.Err()
}

func schema(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func tools() []map[string]any {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	return []map[string]any{
		{
			"name":        "list_agents",
			"description": "List the devices (and their owners and coding agents) reachable through CollabOS, with online status. Targets for call_agent are an owner name (e.g. zhangsan) or a device name.",
			"inputSchema": schema(map[string]any{}),
		},
		{
			"name": "call_agent",
			"description": "Ask the coding agent on another person's/device's machine to do something (e.g. review your changes, investigate, write code) and wait for its answer. " +
				"By default your repository's current changes (uncommitted + untracked + commits since the upstream/merge-base) are sent as a patch with the remote URL and branch, so the remote agent can see the code. " +
				"Blocks until the remote agent finishes (may take minutes). If it returns 'still running', call get_call_result with the call id.",
			"inputSchema": schema(map[string]any{
				"target":          str("Owner name (e.g. \"zhangsan\") or device name from list_agents."),
				"prompt":          str("What the remote agent should do. Be specific; it does not see your conversation."),
				"cwd":             str("Absolute path of the repository whose changes to share. Defaults to the MCP server's working directory."),
				"include_code":    map[string]any{"type": "boolean", "description": "Attach the git context and patch (default true)."},
				"base":            str("Optional git revision the patch is computed against (default: merge-base with upstream, else HEAD)."),
				"timeout_minutes": map[string]any{"type": "number", "description": "How long the remote agent may work (default 30)."},
			}, "target", "prompt"),
		},
		{
			"name":        "get_call_result",
			"description": "Get the status/result of a call started with call_agent, waiting up to wait_seconds (default 600) for it to finish.",
			"inputSchema": schema(map[string]any{
				"call_id":      str("The call id."),
				"wait_seconds": map[string]any{"type": "number", "description": "Max seconds to wait (default 600, 0 = don't wait)."},
			}, "call_id"),
		},
		{
			"name":        "cancel_call",
			"description": "Cancel a running call.",
			"inputSchema": schema(map[string]any{"call_id": str("The call id.")}, "call_id"),
		},
	}
}

func textResult(text string, isErr bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}
}

func (s *Server) toolCall(ctx context.Context, r req) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Meta      struct {
			ProgressToken json.RawMessage `json:"progressToken"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(r.Params, &p); err != nil {
		s.fail(r.ID, -32602, err.Error())
		return
	}
	n := 0
	progress := func(msg string) {
		if len(p.Meta.ProgressToken) == 0 {
			return
		}
		n++
		s.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress",
			"params": map[string]any{"progressToken": p.Meta.ProgressToken, "progress": n, "message": msg}})
	}
	text, err := s.dispatch(ctx, p.Name, p.Arguments, progress)
	if err != nil {
		s.reply(r.ID, textResult(err.Error(), true))
		return
	}
	s.reply(r.ID, textResult(text, false))
}

type callArgs struct {
	Target         string   `json:"target"`
	Prompt         string   `json:"prompt"`
	Cwd            string   `json:"cwd"`
	IncludeCode    *bool    `json:"include_code"`
	Base           string   `json:"base"`
	TimeoutMinutes float64  `json:"timeout_minutes"`
	CallID         string   `json:"call_id"`
	WaitSeconds    *float64 `json:"wait_seconds"`
}

func (s *Server) dispatch(ctx context.Context, name string, raw json.RawMessage, progress func(string)) (string, error) {
	var a callArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", err
		}
	}
	switch name {
	case "list_agents":
		ds, err := s.API.Agents(ctx)
		if err != nil {
			return "", err
		}
		return FormatAgents(ds), nil
	case "call_agent":
		return s.callAgent(ctx, a, progress)
	case "get_call_result":
		wait := 600.0
		if a.WaitSeconds != nil {
			wait = *a.WaitSeconds
		}
		c, err := s.API.Wait(ctx, a.CallID, time.Now().Add(time.Duration(wait)*time.Second), func(c wire.Call) { progress(progressLine(c)) })
		if err != nil {
			return "", err
		}
		if c.ID == "" {
			c, err = s.API.Get(ctx, a.CallID, 0, "")
			if err != nil {
				return "", err
			}
		}
		return FormatCall(c), nil
	case "cancel_call":
		c, err := s.API.Cancel(ctx, a.CallID)
		if err != nil {
			return "", err
		}
		return FormatCall(c), nil
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func (s *Server) callAgent(ctx context.Context, a callArgs, progress func(string)) (string, error) {
	in := wire.CallRequest{Target: a.Target, Prompt: a.Prompt, TimeoutSec: int(a.TimeoutMinutes * 60)}
	if a.IncludeCode == nil || *a.IncludeCode {
		dir := a.Cwd
		if dir == "" {
			dir = s.Cwd
		}
		if cc, err := gitctx.Capture(ctx, dir, a.Base); err == nil {
			in.Context = cc
		} else {
			in.Context = &wire.CodeContext{Note: "caller had no git context: " + err.Error()}
		}
	}
	c, err := s.API.Create(ctx, in)
	if err != nil {
		return "", err
	}
	progress(fmt.Sprintf("call %s sent to %s/%s", c.ID, c.ToOwner, c.ToDevice))
	limit := time.Duration(c.TimeoutSec)*time.Second + 30*time.Second
	if limit > s.MaxWait {
		limit = s.MaxWait
	}
	done, err := s.API.Wait(ctx, c.ID, time.Now().Add(limit), func(c wire.Call) { progress(progressLine(c)) })
	if ctx.Err() != nil {
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _ = s.API.Cancel(cctx, c.ID)
		cancel()
		return "", ctx.Err()
	}
	if err != nil {
		return fmt.Sprintf("Call %s was sent but polling failed (%v). Use get_call_result with call_id=%s.", c.ID, err, c.ID), nil
	}
	if done.ID == "" {
		done = c
	}
	return FormatCall(done), nil
}

func progressLine(c wire.Call) string {
	s := c.Status
	if c.Progress != "" {
		s += ": " + c.Progress
	}
	return s
}

// FormatAgents renders the device list for an agent.
func FormatAgents(ds []wire.Device) string {
	if len(ds) == 0 {
		return "No devices are registered."
	}
	var b strings.Builder
	for _, d := range ds {
		st := "offline"
		if d.Online {
			st = "online"
		}
		self := ""
		if d.Self {
			self = " (this device)"
		}
		fmt.Fprintf(&b, "- %s — owner %s, device %s, %s%s", d.Owner, d.Owner, d.Name, st, self)
		if d.Agent != "" {
			fmt.Fprintf(&b, ", agent %s", d.Agent)
		}
		if d.OS != "" {
			fmt.Fprintf(&b, ", %s", d.OS)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// FormatCall renders a call for an agent.
func FormatCall(c wire.Call) string {
	took := ""
	if c.FinishedAt != nil {
		took = ", took " + c.FinishedAt.Sub(c.CreatedAt).Round(time.Second).String()
	}
	head := fmt.Sprintf("[call %s → %s/%s: %s%s]", c.ID, c.ToOwner, c.ToDevice, c.Status, took)
	switch c.Status {
	case wire.StatusSucceeded:
		return head + "\n\nReply from " + c.ToOwner + "'s agent:\n\n" + c.Result
	case wire.StatusFailed, wire.StatusCanceled:
		out := head + "\nError: " + c.Error
		if c.Result != "" {
			out += "\n\nPartial reply:\n" + c.Result
		}
		return out
	}
	return head + "\nStill running (" + progressLine(c) + "). Call get_call_result with call_id=" + c.ID + " to keep waiting."
}

// Cwd returns the process working directory.
func Cwd() string {
	d, _ := os.Getwd()
	return d
}

// CallAgent is call_agent for the CLI.
func (s *Server) CallAgent(ctx context.Context, target, prompt, cwd string, includeCode bool, timeoutMin float64, progress func(string)) (string, error) {
	return s.callAgent(ctx, callArgs{Target: target, Prompt: prompt, Cwd: cwd, IncludeCode: &includeCode, TimeoutMinutes: timeoutMin}, progress)
}
