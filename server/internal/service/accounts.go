package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/auth"
	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

var handleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func randCode(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// LoginInput is the dev login request.
type LoginInput struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Handle     string `json:"handle"`
	InviteCode string `json:"invite_code"`
	Password   string `json:"password"`
}

// LoginResult is returned by DevLogin.
type LoginResult struct {
	Token       string `json:"token"`
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Handle      string `json:"handle"`
	WorkspaceID string `json:"workspace_id"`
}

// DevLogin creates or reuses a user by email and issues a 90-day token. With
// an invite code the user joins that workspace; otherwise a personal
// workspace is created on first login. Only enabled when COLLAB_DEV_LOGIN=true.
func (s *Service) DevLogin(ctx context.Context, in LoginInput) (LoginResult, error) {
	if !s.Cfg.DevLogin {
		return LoginResult{}, apperr.ErrDevLoginOff
	}
	if s.Cfg.DevLoginPassword != "" && subtle.ConstantTimeCompare([]byte(in.Password), []byte(s.Cfg.DevLoginPassword)) != 1 {
		return LoginResult{}, apperr.ErrBadPassword
	}
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Email == "" || !strings.Contains(in.Email, "@") {
		return LoginResult{}, apperr.BadRequest("a valid email is required")
	}
	if s.Cfg.DevLoginExistingOnly {
		ex, err := s.Q.GetUserByEmail(ctx, in.Email)
		if err != nil {
			return LoginResult{}, apperr.ErrUnknownAccount
		}
		// Existing-only mode never lets a login rename an account or join other workspaces.
		in.Name, in.Handle, in.InviteCode = ex.Name, ex.Handle, ""
	}
	if in.Name == "" {
		in.Name = strings.Split(in.Email, "@")[0]
	}
	if in.Handle == "" {
		in.Handle = strings.Split(in.Email, "@")[0]
	}
	in.Handle = strings.ToLower(in.Handle)
	if !handleRe.MatchString(in.Handle) {
		return LoginResult{}, apperr.BadRequest("handle must be 2-32 chars of a-z, 0-9, _ or -")
	}
	u, err := s.Q.UpsertUser(ctx, db.UpsertUserParams{Email: in.Email, Name: in.Name, Handle: in.Handle})
	if err != nil {
		return LoginResult{}, apperr.Conflict("auth.handle_taken", "handle already taken")
	}
	var wsID pgtype.UUID
	if in.InviteCode != "" {
		w, err := s.Q.GetWorkspaceByInvite(ctx, strings.TrimSpace(in.InviteCode))
		if err != nil {
			return LoginResult{}, apperr.BadRequest("invalid invite code")
		}
		if err := s.Q.AddMember(ctx, db.AddMemberParams{WorkspaceID: w.ID, UserID: u.ID, Role: "member"}); err != nil {
			return LoginResult{}, err
		}
		wsID = w.ID
		s.Audit(ctx, w.ID, "user", util.UUIDString(u.ID), "workspace.join", "workspace", util.UUIDString(w.ID), nil)
		s.dock(w.ID, "members")
	} else {
		wss, err := s.Q.ListWorkspacesForUser(ctx, u.ID)
		if err != nil {
			return LoginResult{}, err
		}
		if len(wss) == 0 {
			w, err := s.Q.CreateWorkspace(ctx, db.CreateWorkspaceParams{Name: in.Name + " 的工作区", InviteCode: randCode(6)})
			if err != nil {
				return LoginResult{}, err
			}
			if err := s.Q.AddMember(ctx, db.AddMemberParams{WorkspaceID: w.ID, UserID: u.ID, Role: "owner"}); err != nil {
				return LoginResult{}, err
			}
			wsID = w.ID
			s.Audit(ctx, w.ID, "user", util.UUIDString(u.ID), "workspace.create", "workspace", util.UUIDString(w.ID), nil)
		} else {
			wsID = wss[0].ID
		}
	}
	tok := auth.NewToken(auth.UserTokenPrefix)
	if err := s.Q.CreateAuthToken(ctx, db.CreateAuthTokenParams{
		TokenHash: auth.Hash(tok), UserID: u.ID,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(90 * 24 * time.Hour), Valid: true},
	}); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: tok, UserID: util.UUIDString(u.ID), Name: u.Name, Email: u.Email, Handle: u.Handle, WorkspaceID: util.UUIDString(wsID)}, nil
}

// Authenticate resolves a user token.
func (s *Service) Authenticate(ctx context.Context, token string) (auth.Actor, error) {
	u, err := s.Q.GetUserByToken(ctx, auth.Hash(token))
	if err != nil {
		return auth.Actor{}, apperr.ErrUnauthorized
	}
	return auth.Actor{UserID: util.UUIDString(u.ID), Email: u.Email, Name: u.Name}, nil
}

// RequireMember checks workspace membership and returns the role.
func (s *Service) RequireMember(ctx context.Context, wsID, userID string) (string, error) {
	w, err := util.ParseUUID(wsID)
	if err != nil {
		return "", apperr.ErrForbidden
	}
	u, err := util.ParseUUID(userID)
	if err != nil {
		return "", apperr.ErrForbidden
	}
	role, err := s.Q.GetMemberRole(ctx, db.GetMemberRoleParams{WorkspaceID: w, UserID: u})
	if err != nil {
		return "", apperr.ErrForbidden
	}
	return role, nil
}

// WorkspaceDTO is a workspace the user belongs to.
type WorkspaceDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	InviteCode string `json:"invite_code"`
	Role       string `json:"role"`
}

// Me describes the current user.
type Me struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Email      string         `json:"email"`
	Handle     string         `json:"handle"`
	Workspaces []WorkspaceDTO `json:"workspaces"`
}

// GetMe returns the user and their workspaces.
func (s *Service) GetMe(ctx context.Context, userID string) (Me, error) {
	uid := util.MustUUID(userID)
	u, err := s.Q.GetUser(ctx, uid)
	if err != nil {
		return Me{}, apperr.ErrUnauthorized
	}
	rows, err := s.Q.ListWorkspacesForUser(ctx, uid)
	if err != nil {
		return Me{}, err
	}
	me := Me{ID: userID, Name: u.Name, Email: u.Email, Handle: u.Handle, Workspaces: []WorkspaceDTO{}}
	for _, r := range rows {
		me.Workspaces = append(me.Workspaces, WorkspaceDTO{util.UUIDString(r.ID), r.Name, r.InviteCode, r.Role})
	}
	return me, nil
}

// MemberDTO is a workspace member.
type MemberDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

// ListMembers lists workspace members.
func (s *Service) ListMembers(ctx context.Context, wsID string) ([]MemberDTO, error) {
	rows, err := s.Q.ListMembers(ctx, util.MustUUID(wsID))
	if err != nil {
		return nil, err
	}
	out := make([]MemberDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, MemberDTO{util.UUIDString(r.ID), r.Name, r.Email, r.Handle, r.Role})
	}
	return out, nil
}

// ---------- projects ----------

// ProjectInput creates a project.
type ProjectInput struct {
	Name          string `json:"name"`
	RemoteURL     string `json:"remote_url"`
	DefaultBranch string `json:"default_branch"`
	Provider      string `json:"provider"`
	LeadID        string `json:"lead_id"`
}

// CreateProject creates a project and its room.
func (s *Service) CreateProject(ctx context.Context, wsID, actorID string, in ProjectInput) (ProjectDTO, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.RemoteURL) == "" {
		return ProjectDTO{}, apperr.BadRequest("name and remote_url are required")
	}
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	if in.Provider == "" {
		in.Provider = "local"
	}
	if in.Provider != "local" && in.Provider != "github" {
		return ProjectDTO{}, apperr.BadRequest("provider must be local or github")
	}
	if in.LeadID == "" {
		in.LeadID = actorID
	}
	ws := util.MustUUID(wsID)
	lead, err := util.ParseUUID(in.LeadID)
	if err != nil {
		return ProjectDTO{}, apperr.BadRequest("invalid lead_id")
	}
	if _, err := s.Q.GetMemberRole(ctx, db.GetMemberRoleParams{WorkspaceID: ws, UserID: lead}); err != nil {
		return ProjectDTO{}, apperr.BadRequest("lead must be a workspace member")
	}
	p, err := s.Q.CreateProject(ctx, db.CreateProjectParams{WorkspaceID: ws, Name: in.Name, RemoteUrl: strings.TrimSpace(in.RemoteURL), DefaultBranch: in.DefaultBranch, Provider: in.Provider, LeadID: lead})
	if err != nil {
		return ProjectDTO{}, err
	}
	r, err := s.Q.CreateRoom(ctx, db.CreateRoomParams{WorkspaceID: ws, ProjectID: p.ID, Name: in.Name})
	if err != nil {
		return ProjectDTO{}, err
	}
	if _, err := s.Q.CreateMemoryVersion(ctx, db.CreateMemoryVersionParams{ProjectID: p.ID, Content: "# " + in.Name + " MEMORY\n\n", Status: "confirmed", ProposedBy: "system"}); err != nil {
		return ProjectDTO{}, err
	}
	s.Audit(ctx, ws, "user", actorID, "project.create", "project", util.UUIDString(p.ID), in)
	s.dock(ws, "projects")
	return ProjectDTO{util.UUIDString(p.ID), p.Name, p.RemoteUrl, p.DefaultBranch, p.Provider, util.UUIDString(p.LeadID), util.UUIDString(r.ID)}, nil
}

// ListProjects lists projects with their room ids.
func (s *Service) ListProjects(ctx context.Context, wsID string) ([]ProjectDTO, error) {
	ws := util.MustUUID(wsID)
	ps, err := s.Q.ListProjects(ctx, ws)
	if err != nil {
		return nil, err
	}
	rooms, err := s.Q.ListRooms(ctx, ws)
	if err != nil {
		return nil, err
	}
	byProject := map[string]string{}
	for _, r := range rooms {
		byProject[util.UUIDString(r.ProjectID)] = util.UUIDString(r.ID)
	}
	out := make([]ProjectDTO, 0, len(ps))
	for _, p := range ps {
		id := util.UUIDString(p.ID)
		out = append(out, ProjectDTO{id, p.Name, p.RemoteUrl, p.DefaultBranch, p.Provider, util.UUIDString(p.LeadID), byProject[id]})
	}
	return out, nil
}

// ---------- daemons & agents ----------

// AgentInput declares an agent hosted by a daemon.
type AgentInput struct {
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Kind   string `json:"kind"`
	Model  string `json:"model"`
}

// PairInput pairs a new daemon.
type PairInput struct {
	Name   string       `json:"name"`
	Agents []AgentInput `json:"agents"`
}

// PairResult carries the one-time node token.
type PairResult struct {
	Daemon    DaemonDTO  `json:"daemon"`
	NodeToken string     `json:"node_token"`
	Agents    []AgentDTO `json:"agents"`
}

// Pair registers a daemon for the user and returns its node token (shown once).
func (s *Service) Pair(ctx context.Context, wsID, userID string, in PairInput) (PairResult, error) {
	if strings.TrimSpace(in.Name) == "" {
		return PairResult{}, apperr.BadRequest("name is required")
	}
	ws := util.MustUUID(wsID)
	for _, a := range in.Agents {
		h := strings.ToLower(a.Handle)
		if !handleRe.MatchString(h) {
			return PairResult{}, apperr.BadRequest("invalid agent handle " + a.Handle)
		}
		if _, err := s.Q.GetAgentByHandle(ctx, db.GetAgentByHandleParams{WorkspaceID: ws, Handle: h}); err == nil {
			return PairResult{}, apperr.Conflict("agent.handle_taken", "agent handle already taken: "+h)
		}
	}
	tok := auth.NewToken(auth.NodeTokenPrefix)
	d, err := s.Q.CreateDaemon(ctx, db.CreateDaemonParams{WorkspaceID: ws, OwnerID: util.MustUUID(userID), Name: in.Name, NodeTokenHash: auth.Hash(tok)})
	if err != nil {
		return PairResult{}, err
	}
	res := PairResult{Daemon: daemonDTO(d, false), NodeToken: tok, Agents: []AgentDTO{}}
	for _, a := range in.Agents {
		ag, err := s.createAgent(ctx, ws, d, a)
		if err != nil {
			return PairResult{}, err
		}
		res.Agents = append(res.Agents, ag)
	}
	s.Audit(ctx, ws, "user", userID, "daemon.pair", "daemon", util.UUIDString(d.ID), map[string]any{"name": in.Name, "agents": in.Agents})
	s.dock(ws, "devices")
	return res, nil
}

func (s *Service) createAgent(ctx context.Context, ws pgtype.UUID, d db.Daemon, a AgentInput) (AgentDTO, error) {
	if a.Kind == "" {
		a.Kind = "specialist"
	}
	if a.Kind != "chief" && a.Kind != "specialist" {
		return AgentDTO{}, apperr.BadRequest("agent kind must be chief or specialist")
	}
	if a.Model == "" {
		a.Model = s.Cfg.DefaultModel
	}
	if a.Name == "" {
		a.Name = a.Handle
	}
	ag, err := s.Q.CreateAgent(ctx, db.CreateAgentParams{WorkspaceID: ws, OwnerID: d.OwnerID, DaemonID: d.ID, Name: a.Name, Handle: strings.ToLower(a.Handle), Kind: a.Kind, Runtime: "opencode", Model: a.Model})
	if err != nil {
		return AgentDTO{}, apperr.Conflict("agent.handle_taken", "agent handle already taken")
	}
	return s.agentDTO(ag), nil
}

// AddAgent adds an agent to one of the user's daemons.
func (s *Service) AddAgent(ctx context.Context, wsID, userID, daemonID string, in AgentInput) (AgentDTO, error) {
	d, err := s.ownDaemon(ctx, wsID, userID, daemonID)
	if err != nil {
		return AgentDTO{}, err
	}
	if !handleRe.MatchString(strings.ToLower(in.Handle)) {
		return AgentDTO{}, apperr.BadRequest("invalid handle")
	}
	ag, err := s.createAgent(ctx, d.WorkspaceID, d, in)
	if err == nil {
		s.Audit(ctx, d.WorkspaceID, "user", userID, "agent.create", "agent", ag.ID, in)
		s.dock(d.WorkspaceID, "devices")
	}
	return ag, err
}

func (s *Service) ownDaemon(ctx context.Context, wsID, userID, daemonID string) (db.Daemon, error) {
	id, err := util.ParseUUID(daemonID)
	if err != nil {
		return db.Daemon{}, apperr.ErrNotFound
	}
	d, err := s.Q.GetDaemon(ctx, id)
	if err != nil || util.UUIDString(d.WorkspaceID) != wsID {
		return db.Daemon{}, apperr.ErrNotFound
	}
	if util.UUIDString(d.OwnerID) != userID {
		return db.Daemon{}, apperr.ErrForbidden
	}
	return d, nil
}

// IntakeInput updates an agent's intake policy (owner only).
type IntakeInput struct {
	Mode       string   `json:"mode"`
	Allowlist  []string `json:"allowlist"`
	DailyQuota int32    `json:"daily_quota"`
}

// UpdateIntake changes intake policy for the owner's agent.
func (s *Service) UpdateIntake(ctx context.Context, wsID, userID, agentID string, in IntakeInput) (AgentDTO, error) {
	a, err := s.getAgent(ctx, wsID, agentID)
	if err != nil {
		return AgentDTO{}, err
	}
	if util.UUIDString(a.OwnerID) != userID {
		return AgentDTO{}, apperr.ErrForbidden
	}
	switch in.Mode {
	case delegation.IntakeManual, delegation.IntakeAllowlist, delegation.IntakeClosed:
	default:
		return AgentDTO{}, apperr.BadRequest("mode must be manual, allowlist or closed")
	}
	if in.Allowlist == nil {
		in.Allowlist = []string{}
	}
	if in.DailyQuota < 0 {
		return AgentDTO{}, apperr.BadRequest("daily_quota must be >= 0")
	}
	up, err := s.Q.UpdateIntakePolicy(ctx, db.UpdateIntakePolicyParams{ID: a.ID, IntakeMode: in.Mode, IntakeAllowlist: util.JSON(in.Allowlist), DailyQuota: in.DailyQuota})
	if err != nil {
		return AgentDTO{}, err
	}
	s.Audit(ctx, a.WorkspaceID, "user", userID, "agent.intake_policy", "agent", agentID, in)
	s.dock(a.WorkspaceID, "devices")
	return s.agentDTO(up), nil
}

func (s *Service) getAgent(ctx context.Context, wsID, agentID string) (db.Agent, error) {
	id, err := util.ParseUUID(agentID)
	if err != nil {
		return db.Agent{}, apperr.ErrNotFound
	}
	a, err := s.Q.GetAgent(ctx, id)
	if err != nil || util.UUIDString(a.WorkspaceID) != wsID {
		return db.Agent{}, apperr.ErrNotFound
	}
	return a, nil
}

// ListAgents lists agents in the workspace.
func (s *Service) ListAgents(ctx context.Context, wsID string) ([]AgentDTO, error) {
	rows, err := s.Q.ListAgents(ctx, util.MustUUID(wsID))
	if err != nil {
		return nil, err
	}
	out := make([]AgentDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, s.agentDTO(a))
	}
	return out, nil
}

// AuthenticateDaemon resolves a node token.
func (s *Service) AuthenticateDaemon(ctx context.Context, token string) (db.Daemon, error) {
	d, err := s.Q.GetDaemonByToken(ctx, auth.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return db.Daemon{}, apperr.ErrUnauthorized
	}
	return d, nil
}

// ListDaemons lists daemons with live status.
func (s *Service) ListDaemons(ctx context.Context, wsID string) ([]DaemonDTO, error) {
	rows, err := s.Q.ListDaemons(ctx, util.MustUUID(wsID))
	if err != nil {
		return nil, err
	}
	out := make([]DaemonDTO, 0, len(rows))
	for _, d := range rows {
		out = append(out, daemonDTO(d, s.Daemons.Online(util.UUIDString(d.ID))))
	}
	return out, nil
}

func daemonCaps(d db.Daemon) protocol.Capabilities {
	var c protocol.Capabilities
	_ = util.Unmarshal(d.Capabilities, &c)
	return c
}
