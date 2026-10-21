// Package handler wires HTTP and WebSocket routes to the service layer.
// Handlers only parse input, call the service and serialize output.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/auth"
	"github.com/collabos/collabos/server/internal/logger"
	"github.com/collabos/collabos/server/internal/service"
	"github.com/collabos/collabos/server/internal/wsconn"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Handler holds dependencies.
type Handler struct {
	S        *service.Service
	log      *slog.Logger
	upgrader websocket.Upgrader
	WebDir   string
}

// New creates the handler.
func New(s *service.Service, webDir string) *Handler {
	h := &Handler{S: s, log: logger.New("http"), WebDir: webDir}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize: 4096, WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			o := r.Header.Get("Origin")
			return o == "" || o == s.Cfg.AllowedOrigin || sameHost(o, r.Host)
		},
	}
	return h
}

func sameHost(origin, host string) bool {
	o := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return o == host
}

// Router builds the chi router.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, h.cors)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	r.Post("/api/auth/dev-login", h.devLogin)
	r.Get("/api/config", h.publicConfig)
	r.Get("/ws", h.uiWS)
	r.Get("/daemon/ws", h.daemonWS)
	r.Group(func(r chi.Router) {
		r.Use(h.requireUser)
		r.Get("/api/me", h.me)
		r.Group(func(r chi.Router) {
			r.Use(h.requireWorkspace)
			r.Get("/api/members", h.listMembers)
			r.Get("/api/projects", h.listProjects)
			r.Post("/api/projects", h.createProject)
			r.Get("/api/projects/{id}/memory", h.listMemory)
			r.Post("/api/memory/{id}/decide", h.decideMemory)
			r.Get("/api/rooms", h.listRooms)
			r.Get("/api/rooms/{id}/messages", h.listMessages)
			r.Post("/api/rooms/{id}/messages", h.sendMessage)
			r.Post("/api/messages/{id}/confirm-draft", h.confirmDraft)
			r.Post("/api/messages/{id}/discard-draft", h.discardDraft)
			r.Get("/api/agents", h.listAgents)
			r.Patch("/api/agents/{id}/intake", h.updateIntake)
			r.Get("/api/daemons", h.listDaemons)
			r.Post("/api/daemons", h.pair)
			r.Post("/api/daemons/{id}/agents", h.addAgent)
			r.Get("/api/tasks", h.listTasks)
			r.Post("/api/tasks", h.createDelegation)
			r.Get("/api/tasks/{id}", h.getTask)
			r.Get("/api/tasks/{id}/events", h.listEvents)
			r.Post("/api/tasks/{id}/intake", h.decideIntake)
			r.Post("/api/tasks/{id}/cancel", h.cancelTask)
			r.Post("/api/permissions/{id}/decide", h.decidePermission)
			r.Get("/api/pull-requests", h.listPRs)
			r.Post("/api/approvals/{id}/decide", h.decideMerge)
			r.Get("/api/audit", h.listAudit)
		})
	})
	if h.WebDir != "" {
		r.NotFound(h.spa)
	}
	return r
}

func (h *Handler) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, apperr.ErrNotFound)
		return
	}
	p := filepath.Join(h.WebDir, filepath.Clean("/"+r.URL.Path))
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		http.ServeFile(w, r, p)
		return
	}
	http.ServeFile(w, r, filepath.Join(h.WebDir, "index.html"))
}

func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o == h.S.Cfg.AllowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Workspace-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) string {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	return ""
}

func (h *Handler) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := h.S.Authenticate(r.Context(), bearer(r))
		if err != nil {
			writeErr(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithActor(r.Context(), a)))
	})
}

func (h *Handler) requireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, _ := auth.ActorFrom(r.Context())
		ws := r.Header.Get("X-Workspace-ID")
		if _, err := h.S.RequireMember(r.Context(), ws, a.UserID); err != nil {
			writeErr(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithWorkspace(r.Context(), ws)))
	})
}

func ids(r *http.Request) (ws, user string) {
	a, _ := auth.ActorFrom(r.Context())
	return auth.WorkspaceFrom(r.Context()), a.UserID
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	e := apperr.As(err)
	if e.Status >= 500 {
		logger.New("http").Error("internal error", "err", err)
	}
	writeJSON(w, e.Status, map[string]string{"error": e.Msg, "code": e.Code})
}

func decode(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return apperr.BadRequest("invalid JSON body")
	}
	return nil
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// ---------- routes ----------

func (h *Handler) devLogin(w http.ResponseWriter, r *http.Request) {
	var in service.LoginInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	res, err := h.S.DevLogin(r.Context(), in)
	respond(w, res, err)
}

func (h *Handler) publicConfig(w http.ResponseWriter, _ *http.Request) {
	respond(w, map[string]bool{"password_required": h.S.Cfg.DevLoginPassword != "", "dev_login": h.S.Cfg.DevLogin}, nil)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	_, u := ids(r)
	m, err := h.S.GetMe(r.Context(), u)
	respond(w, m, err)
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListMembers(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListProjects(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.ProjectInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.CreateProject(r.Context(), ws, u, in)
	respond(w, v, err)
}

func (h *Handler) listMemory(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListMemory(r.Context(), ws, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) decideMemory(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.MemoryDecisionInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.DecideMemory(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListRooms(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	v, err := h.S.ListMessages(r.Context(), ws, chi.URLParam(r, "id"), after)
	respond(w, v, err)
}

func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.SendInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.SendMessage(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) confirmDraft(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var body struct {
		Draft *service.Draft `json:"draft"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	t, dup, err := h.S.ConfirmDraft(r.Context(), ws, u, chi.URLParam(r, "id"), body.Draft)
	respond(w, map[string]any{"task": t, "duplicate": dup}, err)
}

func (h *Handler) discardDraft(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	err := h.S.DiscardDraft(r.Context(), ws, u, chi.URLParam(r, "id"))
	respond(w, map[string]bool{"ok": true}, err)
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListAgents(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) updateIntake(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.IntakeInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.UpdateIntake(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) listDaemons(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListDaemons(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) pair(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.PairInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.Pair(r.Context(), ws, u, in)
	respond(w, v, err)
}

func (h *Handler) addAgent(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.AgentInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.AddAgent(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListTasks(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) createDelegation(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.DelegationInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if k := r.Header.Get("Idempotency-Key"); k != "" && in.IdempotencyKey == "" {
		in.IdempotencyKey = k
	}
	t, dup, err := h.S.CreateDelegation(r.Context(), ws, u, in)
	respond(w, map[string]any{"task": t, "duplicate": dup}, err)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.GetTask(r.Context(), ws, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	v, err := h.S.ListEvents(r.Context(), ws, chi.URLParam(r, "id"), int32(after))
	respond(w, v, err)
}

func (h *Handler) decideIntake(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.IntakeDecision
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.DecideIntake(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) cancelTask(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	v, err := h.S.CancelTask(r.Context(), ws, u, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) decidePermission(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.PermissionDecisionInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.DecidePermission(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) listPRs(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListPullRequests(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) decideMerge(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	var in service.MergeDecisionInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.S.DecideMerge(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.ListAudit(r.Context(), ws)
	respond(w, v, err)
}

// ---------- websockets ----------

// uiWS subscribes a browser to workspace events. The token is passed as a
// query parameter because browsers cannot set WebSocket headers.
func (h *Handler) uiWS(w http.ResponseWriter, r *http.Request) {
	a, err := h.S.Authenticate(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ws := r.URL.Query().Get("workspace_id")
	if _, err := h.S.RequireMember(r.Context(), ws, a.UserID); err != nil {
		writeErr(w, err)
		return
	}
	raw, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := wsconn.Wrap(raw)
	h.S.UI.Join(ws, c)
	c.Send(protocol.Msg(protocol.TypeServerHandshakeAck, protocol.ServerHandshakeAck{
		SessionID: protocol.NewID(), HeartbeatInterval: int(h.S.Cfg.HeartbeatInterval.Seconds()), ServerTime: time.Now().UTC().Format(time.RFC3339),
	}))
	for {
		e, err := c.Read(r.Context())
		if err != nil {
			return
		}
		if e.Type == protocol.TypeHeartbeatPing {
			c.Send(protocol.Msg(protocol.TypeHeartbeatPong, map[string]any{}))
		}
	}
}

// daemonWS is the daemon's outbound connection, authenticated by node token.
func (h *Handler) daemonWS(w http.ResponseWriter, r *http.Request) {
	d, err := h.S.AuthenticateDaemon(r.Context(), bearer(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := wsconn.Wrap(raw)
	defer c.Close()
	ctx := context.Background()
	first, err := c.Read(ctx)
	if err != nil || first.Type != protocol.TypeClientHandshake {
		return
	}
	var hs protocol.ClientHandshake
	if err := first.Into(&hs); err != nil || hs.ClientType != protocol.ClientDaemon {
		return
	}
	id := d.ID
	h.S.Daemons.Register(uuidStr(id), c)
	last, after := h.S.DaemonConnected(ctx, d, hs)
	c.Send(protocol.Msg(protocol.TypeServerHandshakeAck, protocol.ServerHandshakeAck{
		SessionID: protocol.NewID(), HeartbeatInterval: int(h.S.Cfg.HeartbeatInterval.Seconds()),
		ServerTime: time.Now().UTC().Format(time.RFC3339), DaemonID: uuidStr(id), LastAckedSeq: last,
	}))
	h.log.Info("daemon connected", "daemon", uuidStr(id), "last_acked_seq", last)
	after()
	defer func() {
		if h.S.Daemons.Unregister(uuidStr(id), c) {
			h.S.DaemonDisconnected(d)
			h.log.Info("daemon disconnected", "daemon", uuidStr(id))
		}
	}()
	for {
		e, err := c.Read(ctx)
		if err != nil {
			return
		}
		for _, reply := range h.S.HandleDaemonFrame(ctx, d, e) {
			c.Send(reply)
		}
	}
}
