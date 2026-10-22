// Package server is the relay: it authenticates devices, keeps one outbound
// WebSocket per device daemon, forwards agent-to-agent calls and records
// every call. There are no approvals: any registered device may call any
// other.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/collabos/collabos/v2/internal/wire"
)

// Limits.
const (
	DefaultTimeout = 30 * time.Minute
	MaxTimeout     = 4 * time.Hour
	MaxPromptBytes = 64 << 10
	MaxPatchBytes  = 512 << 10
	MaxResultBytes = 256 << 10
)

type conn struct {
	ws   *websocket.Conn
	mu   sync.Mutex
	meta wire.Frame
	at   time.Time
}

func (c *conn) send(f wire.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return c.ws.WriteJSON(f)
}

// Server is the relay.
type Server struct {
	Store Store
	Log   *slog.Logger

	mu      sync.Mutex
	conns   map[string]*conn
	waiters map[string][]chan struct{}
	callMu  sync.Mutex // serialises read-modify-write of calls
	up      websocket.Upgrader
}

// New returns a relay over store.
func New(st Store, log *slog.Logger) *Server {
	return &Server{Store: st, Log: log, conns: map[string]*conn{}, waiters: map[string][]chan struct{}{},
		up: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("GET /ws", s.auth(s.daemonWS))
	m.HandleFunc("GET /api/me", s.auth(func(w http.ResponseWriter, r *http.Request, d DeviceRow) {
		writeJSON(w, 200, map[string]string{"device": d.Name, "owner": d.Owner})
	}))
	m.HandleFunc("GET /api/agents", s.auth(s.listAgents))
	m.HandleFunc("POST /api/calls", s.auth(s.createCall))
	m.HandleFunc("GET /api/calls", s.auth(s.listCalls))
	m.HandleFunc("GET /api/calls/{id}", s.auth(s.getCall))
	m.HandleFunc("POST /api/calls/{id}/cancel", s.auth(s.cancelCall))
	return m
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) auth(h func(http.ResponseWriter, *http.Request, DeviceRow)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" || tok == r.Header.Get("Authorization") {
			writeErr(w, 401, "missing bearer device token")
			return
		}
		d, err := s.Store.DeviceByToken(r.Context(), HashToken(tok))
		if err != nil {
			writeErr(w, 401, "unknown device token")
			return
		}
		h(w, r, d)
	}
}

// ---------- daemon socket ----------

func (s *Server) daemonWS(w http.ResponseWriter, r *http.Request, d DeviceRow) {
	ws, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(4 << 20)
	var hello wire.Frame
	_ = ws.SetReadDeadline(time.Now().Add(20 * time.Second))
	if err := ws.ReadJSON(&hello); err != nil || hello.Type != "hello" {
		_ = ws.Close()
		return
	}
	c := &conn{ws: ws, meta: hello, at: time.Now()}
	s.mu.Lock()
	if old := s.conns[d.Name]; old != nil {
		_ = old.ws.Close()
	}
	s.conns[d.Name] = c
	s.mu.Unlock()
	_ = s.Store.TouchDevice(context.Background(), d.Name)
	s.Log.Info("device online", "device", d.Name, "owner", d.Owner, "agent", hello.Agent)
	s.reconcile(d.Name, hello.Active)

	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				c.mu.Lock()
				err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
				c.mu.Unlock()
				if err != nil {
					_ = ws.Close()
					return
				}
				_ = s.Store.TouchDevice(context.Background(), d.Name)
			}
		}
	}()
	defer func() {
		close(done)
		_ = ws.Close()
		s.mu.Lock()
		if s.conns[d.Name] == c {
			delete(s.conns, d.Name)
			s.Log.Info("device offline", "device", d.Name)
		}
		s.mu.Unlock()
	}()
	for {
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		var f wire.Frame
		if err := ws.ReadJSON(&f); err != nil {
			return
		}
		s.onFrame(d, f)
	}
}

// reconcile fails calls the reconnecting daemon no longer runs.
func (s *Server) reconcile(device string, active []string) {
	ctx := context.Background()
	open, err := s.Store.OpenCallsFor(ctx, device)
	if err != nil {
		return
	}
	run := map[string]bool{}
	for _, id := range active {
		run[id] = true
	}
	for _, c := range open {
		if c.Status == wire.StatusRunning && !run[c.ID] {
			s.update(c.ID, func(c *wire.Call) { finish(c, wire.StatusFailed, "", "callee daemon restarted while the call was running") })
		}
		if c.Status == wire.StatusQueued {
			s.deliver(c)
		}
	}
}

func (s *Server) onFrame(d DeviceRow, f wire.Frame) {
	switch f.Type {
	case "progress":
		s.update(f.ID, func(c *wire.Call) {
			if c.ToDevice != d.Name || wire.Terminal(c.Status) {
				return
			}
			if c.Status == wire.StatusQueued {
				now := time.Now().UTC()
				c.Status, c.StartedAt = wire.StatusRunning, &now
			}
			c.Progress = truncate(f.Text, 2000)
		})
	case "result":
		s.update(f.ID, func(c *wire.Call) {
			if c.ToDevice != d.Name || wire.Terminal(c.Status) {
				return
			}
			st := f.Status
			if !wire.Terminal(st) {
				st = wire.StatusFailed
			}
			finish(c, st, truncate(f.Text, MaxResultBytes), f.Error)
		})
	}
}

func finish(c *wire.Call, status, result, errMsg string) {
	now := time.Now().UTC()
	c.Status, c.Result, c.Error, c.FinishedAt = status, result, errMsg, &now
	if c.StartedAt == nil {
		c.StartedAt = &now
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]"
}

// update applies fn to a stored call, saves it and wakes waiters.
func (s *Server) update(id string, fn func(*wire.Call)) {
	ctx := context.Background()
	s.callMu.Lock()
	c, err := s.Store.Call(ctx, id)
	if err != nil {
		s.callMu.Unlock()
		return
	}
	before := c.Status + c.Progress
	fn(&c)
	changed := c.Status+c.Progress != before
	if changed {
		if err := s.Store.SaveCall(ctx, c); err != nil {
			s.Log.Error("save call", "id", id, "err", err)
		}
	}
	s.callMu.Unlock()
	if changed {
		s.wake(id)
		if wire.Terminal(c.Status) {
			s.Log.Info("call finished", "id", id, "status", c.Status, "from", c.FromDevice, "to", c.ToDevice)
		}
	}
}

func (s *Server) wake(id string) {
	s.mu.Lock()
	ws := s.waiters[id]
	delete(s.waiters, id)
	s.mu.Unlock()
	for _, ch := range ws {
		close(ch)
	}
}

func (s *Server) waitChan(id string) chan struct{} {
	ch := make(chan struct{})
	s.mu.Lock()
	s.waiters[id] = append(s.waiters[id], ch)
	s.mu.Unlock()
	return ch
}

func (s *Server) deliver(c wire.Call) bool {
	s.mu.Lock()
	cn := s.conns[c.ToDevice]
	s.mu.Unlock()
	if cn == nil {
		return false
	}
	cc := c
	return cn.send(wire.Frame{Type: "call", Call: &cc}) == nil
}

// ---------- API ----------

func (s *Server) devices(ctx context.Context, self string) ([]wire.Device, error) {
	rows, err := s.Store.Devices(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]wire.Device, 0, len(rows))
	for _, r := range rows {
		d := wire.Device{Name: r.Name, Owner: r.Owner, LastSeen: r.LastSeen, Self: r.Name == self}
		if c := s.conns[r.Name]; c != nil {
			d.Online, d.Agent, d.Hostname, d.OS = true, c.meta.Agent, c.meta.Hostname, c.meta.OS
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request, d DeviceRow) {
	ds, err := s.devices(r.Context(), d.Name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ds)
}

// Resolve picks the target device for a name: an exact device name wins,
// otherwise an owner name (the caller's own device is skipped, online
// devices are preferred).
func Resolve(ds []wire.Device, target, self string) (wire.Device, error) {
	t := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(target, "@")))
	if t == "" {
		return wire.Device{}, errors.New("target is required")
	}
	for _, d := range ds {
		if strings.ToLower(d.Name) == t {
			return d, nil
		}
	}
	var best *wire.Device
	for i := range ds {
		d := ds[i]
		if strings.ToLower(d.Owner) != t || d.Name == self {
			continue
		}
		if best == nil || (d.Online && !best.Online) {
			best = &ds[i]
		}
	}
	if best == nil {
		names := []string{}
		for _, d := range ds {
			if d.Name != self {
				names = append(names, d.Owner+"/"+d.Name)
			}
		}
		return wire.Device{}, errors.New("no device or owner named " + strconv.Quote(target) + "; known: " + strings.Join(names, ", "))
	}
	return *best, nil
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "call_" + time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(b)
}

func (s *Server) createCall(w http.ResponseWriter, r *http.Request, d DeviceRow) {
	var in wire.CallRequest
	r.Body = http.MaxBytesReader(w, r.Body, MaxPatchBytes+MaxPromptBytes+64<<10)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > MaxPromptBytes {
		writeErr(w, 400, "prompt is required (max 64 KiB)")
		return
	}
	if in.Context != nil && len(in.Context.Patch) > MaxPatchBytes {
		in.Context.Patch = truncate(in.Context.Patch, MaxPatchBytes)
	}
	ds, err := s.devices(r.Context(), d.Name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	to, err := Resolve(ds, in.Target, d.Name)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	if !to.Online {
		writeErr(w, 409, "device "+to.Name+" ("+to.Owner+") is offline: its collab daemon is not running")
		return
	}
	timeout := time.Duration(in.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	c := wire.Call{ID: newID(), FromDevice: d.Name, FromOwner: d.Owner, ToDevice: to.Name, ToOwner: to.Owner,
		Prompt: in.Prompt, Context: in.Context, TimeoutSec: int(timeout.Seconds()), Status: wire.StatusQueued, CreatedAt: time.Now().UTC()}
	if err := s.Store.SaveCall(r.Context(), c); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.Info("call created", "id", c.ID, "from", c.FromDevice, "to", c.ToDevice)
	if !s.deliver(c) {
		s.update(c.ID, func(c *wire.Call) { finish(c, wire.StatusFailed, "", "could not deliver to "+to.Name) })
	}
	time.AfterFunc(timeout, func() { s.expire(c.ID) })
	c, _ = s.Store.Call(r.Context(), c.ID)
	writeJSON(w, 201, c)
}

func (s *Server) expire(id string) {
	c, err := s.Store.Call(context.Background(), id)
	if err != nil || wire.Terminal(c.Status) {
		return
	}
	s.update(id, func(c *wire.Call) { finish(c, wire.StatusFailed, "", "timed out") })
	s.mu.Lock()
	cn := s.conns[c.ToDevice]
	s.mu.Unlock()
	if cn != nil {
		_ = cn.send(wire.Frame{Type: "cancel", ID: id})
	}
}

func (s *Server) visible(c wire.Call, d DeviceRow) bool {
	return c.FromDevice == d.Name || c.ToDevice == d.Name || c.FromOwner == d.Owner || c.ToOwner == d.Owner
}

// getCall returns a call; ?wait=SECONDS long-polls until it changes or ends.
func (s *Server) getCall(w http.ResponseWriter, r *http.Request, d DeviceRow) {
	id := r.PathValue("id")
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait > 120 {
		wait = 120
	}
	since := r.URL.Query().Get("since") // status+progress the client already has
	deadline := time.After(time.Duration(wait) * time.Second)
	for {
		ch := s.waitChan(id)
		c, err := s.Store.Call(r.Context(), id)
		if err != nil || !s.visible(c, d) {
			writeErr(w, 404, "no such call")
			return
		}
		if wait <= 0 || wire.Terminal(c.Status) || (since != "" && since != c.Status+"|"+c.Progress) {
			writeJSON(w, 200, c)
			return
		}
		select {
		case <-ch:
			since = "\x00" // anything changed: return the next read
		case <-deadline:
			writeJSON(w, 200, c)
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) listCalls(w http.ResponseWriter, r *http.Request, d DeviceRow) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 200 {
		n = 20
	}
	cs, err := s.Store.Calls(r.Context(), d.Name, n)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, cs)
}

func (s *Server) cancelCall(w http.ResponseWriter, r *http.Request, d DeviceRow) {
	id := r.PathValue("id")
	c, err := s.Store.Call(r.Context(), id)
	if err != nil || c.FromDevice != d.Name {
		writeErr(w, 404, "no such call")
		return
	}
	s.update(id, func(c *wire.Call) {
		if !wire.Terminal(c.Status) {
			finish(c, wire.StatusCanceled, "", "canceled by caller")
		}
	})
	s.mu.Lock()
	cn := s.conns[c.ToDevice]
	s.mu.Unlock()
	if cn != nil {
		_ = cn.send(wire.Frame{Type: "cancel", ID: id})
	}
	c, _ = s.Store.Call(r.Context(), id)
	writeJSON(w, 200, c)
}
