package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/config"
	"github.com/collabos/collabos/server/migrations"
)

// newTestService creates a throwaway database from DATABASE_URL, migrates it
// and returns a service. Skips when no Postgres is reachable.
func newTestService(t *testing.T, mut func(*config.Server)) *Service {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	name := fmt.Sprintf("collab_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Skipf("cannot create test database: %v", err)
	}
	pu, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	pu.Path = "/" + name
	u := pu.String()
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	if err := migrations.Up(u); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cfg := config.Server{DatabaseURL: u, DevLogin: true, OfflineGrace: time.Minute, DefaultModel: "test/model"}
	if mut != nil {
		mut(&cfg)
	}
	return New(pool, cfg)
}

func code(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestDevLoginAccessCodeAndExistingOnly(t *testing.T) {
	s := newTestService(t, func(c *config.Server) { c.DevLoginPassword = "letmein" })
	ctx := context.Background()
	if _, err := s.DevLogin(ctx, LoginInput{Email: "a@x.dev"}); code(err) != "auth.bad_password" {
		t.Fatalf("want bad_password, got %v", err)
	}
	a, err := s.DevLogin(ctx, LoginInput{Email: "a@x.dev", Name: "A", Handle: "aa", Password: "letmein"})
	if err != nil {
		t.Fatal(err)
	}
	s.Cfg.DevLoginExistingOnly = true
	if _, err := s.DevLogin(ctx, LoginInput{Email: "new@x.dev", Password: "letmein"}); code(err) != "auth.unknown_account" {
		t.Fatalf("want unknown_account, got %v", err)
	}
	again, err := s.DevLogin(ctx, LoginInput{Email: "a@x.dev", Name: "Mallory", Handle: "mallory", Password: "letmein"})
	if err != nil || again.Handle != "aa" || again.Name != "A" || again.WorkspaceID != a.WorkspaceID {
		t.Fatalf("existing-only login must not rename: %+v %v", again, err)
	}
}

func TestDelegationFlowAuthorization(t *testing.T) {
	s := newTestService(t, nil)
	ctx := context.Background()
	a, err := s.DevLogin(ctx, LoginInput{Email: "a@x.dev", Name: "A", Handle: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	me, err := s.GetMe(ctx, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.DevLogin(ctx, LoginInput{Email: "b@x.dev", Name: "B", Handle: "bob", InviteCode: me.Workspaces[0].InviteCode})
	if err != nil || b.WorkspaceID != a.WorkspaceID {
		t.Fatalf("join: %+v %v", b, err)
	}
	ws := a.WorkspaceID
	if _, err := s.CreateProject(ctx, ws, a.UserID, ProjectInput{Name: "calc", RemoteURL: "/srv/calc.git"}); err != nil {
		t.Fatal(err)
	}
	rooms, err := s.ListRooms(ctx, ws)
	if err != nil || len(rooms) != 1 {
		t.Fatalf("rooms: %v %v", rooms, err)
	}
	mem, err := s.ListMemory(ctx, ws, rooms[0].ProjectID)
	if err != nil || len(mem) != 1 || mem[0].Status != "confirmed" {
		t.Fatalf("initial memory: %+v %v", mem, err)
	}
	if _, err := s.Pair(ctx, ws, b.UserID, PairInput{Name: "bob-laptop", Agents: []AgentInput{{Handle: "bob-coder", Kind: "specialist"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pair(ctx, ws, a.UserID, PairInput{Name: "x", Agents: []AgentInput{{Handle: "bob-coder", Kind: "specialist"}}}); err == nil {
		t.Fatal("duplicate agent handle must fail")
	}

	// A device that never reported a sandbox cannot take cross-owner work.
	tNo, _, err := s.CreateDelegation(ctx, ws, a.UserID, DelegationInput{RoomID: rooms[0].ID, ProjectID: rooms[0].ProjectID, TargetAgentID: agentID(t, s, ws, "bob-coder"), Title: "x", Prompt: "x", IdempotencyKey: "k0"})
	if err != nil || tNo.Status != "rejected" || tNo.ReasonCode != "sandbox_unavailable" {
		t.Fatalf("sandbox: %+v %v", tNo, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE daemons SET capabilities = '{"sandbox":"bubblewrap"}'`); err != nil {
		t.Fatal(err)
	}

	// Mentioning another owner's specialist yields a draft card; client_msg_id dedupes.
	in := SendInput{ClientMsgID: "c1", Body: "@bob-coder add multiply to src/math.ts"}
	m1, err := s.SendMessage(ctx, ws, a.UserID, rooms[0].ID, in)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := s.SendMessage(ctx, ws, a.UserID, rooms[0].ID, in)
	if err != nil || m2.ID != m1.ID {
		t.Fatalf("dedupe: %v %v", m2.ID, err)
	}
	msgs, err := s.ListMessages(ctx, ws, rooms[0].ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var draftID string
	for _, m := range msgs {
		var c struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(m.Card, &c)
		if c.Type == "delegation_draft" {
			draftID = m.ID
		}
	}
	if draftID == "" {
		t.Fatal("no draft card")
	}
	if _, _, err := s.ConfirmDraft(ctx, ws, b.UserID, draftID, nil); err == nil {
		t.Fatal("only the requester may confirm a draft")
	}
	t1, _, err := s.ConfirmDraft(ctx, ws, a.UserID, draftID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t2, _, err := s.ConfirmDraft(ctx, ws, a.UserID, draftID, nil)
	if err != nil || t2.ID != t1.ID {
		t.Fatalf("confirm must be idempotent: %v %v", t2.ID, err)
	}
	if t1.Status != "pending_intake" {
		t.Fatalf("status %s", t1.Status)
	}
	if _, err := s.DecideIntake(ctx, ws, a.UserID, t1.ID, IntakeDecision{Decision: "accept"}); code(err) != "delegation.unauthorized" {
		t.Fatalf("requester must not decide intake: %v", err)
	}
	tr, err := s.DecideIntake(ctx, ws, b.UserID, t1.ID, IntakeDecision{Decision: "reject"})
	if err != nil || tr.Status != "rejected" || tr.ReasonCode != "intake_denied" {
		t.Fatalf("reject: %+v %v", tr, err)
	}

	// The audit log is append-only.
	audit, err := s.ListAudit(ctx, ws)
	if err != nil || len(audit) == 0 {
		t.Fatalf("audit: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE audit_log SET action = 'x'"); err == nil {
		t.Fatal("audit_log UPDATE must be rejected")
	}
	if _, err := s.Pool.Exec(ctx, "DELETE FROM audit_log"); err == nil {
		t.Fatal("audit_log DELETE must be rejected")
	}
}

func agentID(t *testing.T, s *Service, ws, handle string) string {
	t.Helper()
	as, err := s.ListAgents(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range as {
		if a.Handle == handle {
			return a.ID
		}
	}
	t.Fatalf("agent %s not found", handle)
	return ""
}
