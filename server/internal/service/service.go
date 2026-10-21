// Package service holds business logic; handlers only parse and serialize.
package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/collabos/collabos/server/internal/config"
	"github.com/collabos/collabos/server/internal/daemonws"
	"github.com/collabos/collabos/server/internal/logger"
	"github.com/collabos/collabos/server/internal/realtime"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Service is the application core.
type Service struct {
	Pool    *pgxpool.Pool
	Q       *db.Queries
	UI      *realtime.Hub
	Daemons *daemonws.Hub
	Cfg     config.Server
	log     *slog.Logger

	graceMu sync.Mutex
	grace   map[string]*time.Timer // daemon id -> offline grace timer
	taskMu  sync.Mutex             // serializes status transitions
	roomMu  sync.Mutex             // serializes room seq allocation
	frameMu sync.Map               // daemon id -> *sync.Mutex (ordered frame handling)

	warned  sync.Map // task id -> struct{} once the 90% task-budget warning was sent
	nudgeMu sync.Mutex
	nudged  map[string]time.Time // task/user -> last nudge
	// HTTPClient sends outbound IM notifications.
	HTTPClient *http.Client
}

// New builds a service.
func New(pool *pgxpool.Pool, cfg config.Server) *Service {
	cfg.Defaults()
	return &Service{
		Pool: pool, Q: db.New(pool), UI: realtime.NewHub(), Daemons: daemonws.NewHub(),
		Cfg: cfg, log: logger.New("service"), grace: map[string]*time.Timer{},
		nudged: map[string]time.Time{}, HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Service) broadcast(ws pgtype.UUID, typ string, payload any) {
	s.UI.Broadcast(util.UUIDString(ws), protocol.Msg(typ, payload))
}

func (s *Service) dock(ws pgtype.UUID, section string) {
	s.broadcast(ws, protocol.TypeDockChanged, protocol.DockChanged{Section: section})
}

// Audit appends to the append-only audit log. Failures are logged, never fatal.
func (s *Service) Audit(ctx context.Context, ws pgtype.UUID, actorType, actorID, action, targetType, targetID string, detail any) {
	if detail == nil {
		detail = map[string]any{}
	}
	if err := s.Q.AppendAudit(ctx, db.AppendAuditParams{
		WorkspaceID: ws, ActorType: actorType, ActorID: actorID, Action: action,
		TargetType: targetType, TargetID: targetID, Detail: util.JSON(detail),
	}); err != nil {
		s.log.Error("audit append failed", "action", action, "err", err)
	}
}

// AuditEntry is the API shape of an audit row.
type AuditEntry struct {
	ID         int64           `json:"id"`
	ActorType  string          `json:"actor_type"`
	ActorID    string          `json:"actor_id"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  string          `json:"created_at"`
}

// ListAudit returns recent audit entries.
func (s *Service) ListAudit(ctx context.Context, wsID string) ([]AuditEntry, error) {
	ws, err := util.ParseUUID(wsID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListAudit(ctx, db.ListAuditParams{WorkspaceID: ws, Limit: 300})
	if err != nil {
		return nil, err
	}
	out := make([]AuditEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuditEntry{r.ID, r.ActorType, r.ActorID, r.Action, r.TargetType, r.TargetID, raw(r.Detail, "{}"), util.TS(r.CreatedAt)})
	}
	return out, nil
}

func raw(b []byte, def string) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(def)
	}
	return json.RawMessage(b)
}

func auditDTO(r db.AuditLog) AuditEntry {
	return AuditEntry{r.ID, r.ActorType, r.ActorID, r.Action, r.TargetType, r.TargetID, raw(r.Detail, "{}"), util.TS(r.CreatedAt)}
}
