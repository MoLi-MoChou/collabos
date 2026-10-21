package service

import (
	"context"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
)

// Project roles for shared projects (PRD 5.3).
const (
	RoleLead   = "lead"
	RoleWriter = "writer"
	RoleViewer = "viewer"
)

// projectRole returns the caller's role in a project, or "" without access.
// Workspace-visible projects grant every member "writer"; restricted ones
// require an explicit membership.
func (s *Service) projectRole(ctx context.Context, p db.Project, userID string) string {
	if util.UUIDString(p.LeadID) == userID {
		return RoleLead
	}
	if role, err := s.Q.GetProjectMemberRole(ctx, db.GetProjectMemberRoleParams{ProjectID: p.ID, UserID: util.MustUUID(userID)}); err == nil {
		return role
	}
	if p.Visibility == "restricted" {
		return ""
	}
	return RoleWriter
}

// requireProject checks access to the room's project; write=true also
// rejects viewers.
func (s *Service) requireProject(ctx context.Context, r db.Room, userID string, write bool) error {
	if !r.ProjectID.Valid {
		return nil
	}
	p, err := s.Q.GetProject(ctx, r.ProjectID)
	if err != nil {
		return nil
	}
	switch s.projectRole(ctx, p, userID) {
	case "":
		return apperr.Forbidden("project.no_access", "you are not a member of this project")
	case RoleViewer:
		if write {
			return apperr.Forbidden("project.read_only", "viewers cannot post or delegate in this project")
		}
	}
	return nil
}

// ProjectMemberInput adds or changes a project member.
type ProjectMemberInput struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// ProjectMemberDTO is a project member.
type ProjectMemberDTO struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

func (s *Service) leadProject(ctx context.Context, wsID, userID, projectID string) (db.Project, error) {
	pid, err := util.ParseUUID(projectID)
	if err != nil {
		return db.Project{}, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, pid)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return db.Project{}, apperr.ErrNotFound
	}
	if util.UUIDString(p.LeadID) != userID {
		return db.Project{}, apperr.Forbidden("project.not_lead", "only the project lead can manage this project")
	}
	return p, nil
}

// SetProjectMember adds a member (joining inherits MEMORY.md, scenario C).
func (s *Service) SetProjectMember(ctx context.Context, wsID, userID, projectID string, in ProjectMemberInput) ([]ProjectMemberDTO, error) {
	p, err := s.leadProject(ctx, wsID, userID, projectID)
	if err != nil {
		return nil, err
	}
	if in.Role != RoleWriter && in.Role != RoleViewer {
		return nil, apperr.BadRequest("role must be writer or viewer")
	}
	uid, err := util.ParseUUID(in.UserID)
	if err != nil {
		return nil, apperr.BadRequest("invalid user_id")
	}
	if _, err := s.Q.GetMemberRole(ctx, db.GetMemberRoleParams{WorkspaceID: p.WorkspaceID, UserID: uid}); err != nil {
		return nil, apperr.BadRequest("user is not a workspace member")
	}
	if err := s.Q.AddProjectMember(ctx, db.AddProjectMemberParams{ProjectID: p.ID, UserID: uid, Role: in.Role}); err != nil {
		return nil, err
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "project.member_set", "project", projectID, in)
	s.dock(p.WorkspaceID, "projects")
	return s.ListProjectMembers(ctx, wsID, userID, projectID)
}

// RemoveProjectMember removes a member.
func (s *Service) RemoveProjectMember(ctx context.Context, wsID, userID, projectID, memberID string) ([]ProjectMemberDTO, error) {
	p, err := s.leadProject(ctx, wsID, userID, projectID)
	if err != nil {
		return nil, err
	}
	uid, err := util.ParseUUID(memberID)
	if err != nil {
		return nil, apperr.BadRequest("invalid user id")
	}
	if err := s.Q.RemoveProjectMember(ctx, db.RemoveProjectMemberParams{ProjectID: p.ID, UserID: uid}); err != nil {
		return nil, err
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "project.member_remove", "project", projectID, map[string]string{"user_id": memberID})
	s.dock(p.WorkspaceID, "projects")
	return s.ListProjectMembers(ctx, wsID, userID, projectID)
}

// ListProjectMembers lists explicit members (any workspace member may read).
func (s *Service) ListProjectMembers(ctx context.Context, wsID, _ string, projectID string) ([]ProjectMemberDTO, error) {
	pid, err := util.ParseUUID(projectID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, pid)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return nil, apperr.ErrNotFound
	}
	rows, err := s.Q.ListProjectMembers(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectMemberDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectMemberDTO{UserID: util.UUIDString(r.UserID), Name: r.Name, Handle: r.Handle, Role: r.Role})
	}
	return out, nil
}

// ProjectSettingsInput is the lead's project configuration.
type ProjectSettingsInput struct {
	WebhookSecret *string `json:"webhook_secret"`
	TestCommand   string  `json:"test_command"`
	Visibility    string  `json:"visibility"`
}

// UpdateProjectSettings sets webhook secret, test command and visibility.
func (s *Service) UpdateProjectSettings(ctx context.Context, wsID, userID, projectID string, in ProjectSettingsInput) (ProjectDTO, error) {
	p, err := s.leadProject(ctx, wsID, userID, projectID)
	if err != nil {
		return ProjectDTO{}, err
	}
	if in.Visibility == "" {
		in.Visibility = p.Visibility
	}
	if in.Visibility != "workspace" && in.Visibility != "restricted" {
		return ProjectDTO{}, apperr.BadRequest("visibility must be workspace or restricted")
	}
	secret := p.WebhookSecret
	if in.WebhookSecret != nil {
		secret = *in.WebhookSecret
	}
	if len(in.TestCommand) > 500 {
		return ProjectDTO{}, apperr.BadRequest("test_command too long")
	}
	up, err := s.Q.SetProjectSettings(ctx, db.SetProjectSettingsParams{ID: p.ID, WebhookSecret: secret, TestCommand: in.TestCommand, Visibility: in.Visibility})
	if err != nil {
		return ProjectDTO{}, err
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "project.settings", "project", projectID, map[string]any{"test_command": in.TestCommand, "visibility": in.Visibility, "webhook_secret_set": secret != ""})
	s.dock(p.WorkspaceID, "projects")
	return s.projectDTO(up), nil
}

// ListRoomsFor lists rooms the user can see.
func (s *Service) ListRoomsFor(ctx context.Context, wsID, userID string) ([]RoomDTO, error) {
	rows, err := s.Q.ListRooms(ctx, util.MustUUID(wsID))
	if err != nil {
		return nil, err
	}
	out := make([]RoomDTO, 0, len(rows))
	for _, r := range rows {
		if s.requireProject(ctx, r, userID, false) != nil {
			continue
		}
		out = append(out, RoomDTO{util.UUIDString(r.ID), util.UUIDString(r.ProjectID), r.Name, r.LastSeq})
	}
	return out, nil
}

// RoomAccess checks that the user may read (or write) a room.
func (s *Service) RoomAccess(ctx context.Context, wsID, userID, roomID string, write bool) error {
	r, err := s.room(ctx, wsID, roomID)
	if err != nil {
		return err
	}
	return s.requireProject(ctx, r, userID, write)
}
