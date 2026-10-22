package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain doubles as a fake ACP agent when FAKE_ACP=1.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_ACP") == "silent" {
		silentAgent()
		return
	}
	if os.Getenv("FAKE_ACP") == "1" {
		fakeAgent()
		return
	}
	os.Exit(m.Run())
}

func fakeAgent() {
	sc := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	var promptID json.RawMessage
	for sc.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		_ = json.Unmarshal(sc.Bytes(), &m)
		switch m.Method {
		case "initialize":
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"protocolVersion": 1, "agentInfo": map[string]string{"name": "fake", "version": "1"}}})
		case "session/new":
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"sessionId": "s1"}})
		case "session/prompt":
			promptID = m.ID
			upd := func(u map[string]any) {
				out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": u}})
			}
			upd(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "Let me look."}})
			upd(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": "read a.py"})
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "session/request_permission", "params": map[string]any{
				"sessionId": "s1", "toolCall": map[string]any{"toolCallId": "t1"},
				"options": []map[string]string{{"optionId": "no", "kind": "reject_once"}, {"optionId": "yes", "kind": "allow_once"}, {"optionId": "always", "kind": "allow_always"}}}})
		default:
			if string(m.ID) == "99" {
				var r struct {
					Outcome struct {
						OptionID string `json:"optionId"`
					} `json:"outcome"`
				}
				_ = json.Unmarshal(m.Result, &r)
				upd := func(u map[string]any) {
					out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": u}})
				}
				upd(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "Review: "}})
				upd(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": fmt.Sprintf("ok (permission=%s)", r.Outcome.OptionID)}})
				out.Encode(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]any{"stopReason": "end_turn"}})
			}
		}
	}
}

func TestRunBypassAndFinalMessage(t *testing.T) {
	exe, _ := os.Executable()
	var prog []string
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := Run(ctx, Job{Argv: []string{exe}, Dir: t.TempDir(), Env: []string{"FAKE_ACP=1"}, Prompt: "review", Progress: func(s string) { prog = append(prog, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Review: ok (permission=always)" || res.StopReason != "end_turn" || res.Agent != "fake 1" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(strings.Join(prog, "|"), "tool: read a.py") {
		t.Fatal(prog)
	}
}

func TestRunMissingAgent(t *testing.T) {
	_, err := Run(context.Background(), Job{Argv: []string{"definitely-not-an-agent-xyz"}, Dir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error")
	}
}

// silentAgent starts a session, then never answers the prompt.
func silentAgent() {
	sc := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(sc.Bytes(), &m)
		switch m.Method {
		case "initialize":
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"protocolVersion": 1}})
		case "session/new":
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"sessionId": "s1"}})
		}
	}
}

func TestInactivityFailsFast(t *testing.T) {
	exe, _ := os.Executable()
	t0 := time.Now()
	_, err := Run(context.Background(), Job{Argv: []string{exe}, Dir: t.TempDir(), Env: []string{"FAKE_ACP=silent"}, Prompt: "x", Inactivity: 500 * time.Millisecond})
	if !errors.Is(err, ErrInactive) || !strings.Contains(err.Error(), "no ACP activity") {
		t.Fatalf("want inactivity error, got %v", err)
	}
	if time.Since(t0) > 5*time.Second {
		t.Fatal("took too long")
	}
}
