package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Card is the structured payload of an interactive chat message.
type Card map[string]any

// Mention is one resolved @mention.
type Mention struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Kind   string `json:"kind,omitempty"`
}

var mentionRe = regexp.MustCompile(`@([a-zA-Z0-9][a-zA-Z0-9_-]{1,31})`)

// ParseMentionHandles returns unique lower-case handles mentioned in body.
func ParseMentionHandles(body string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range mentionRe.FindAllStringSubmatch(body, -1) {
		h := strings.ToLower(m[1])
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

func (s *Service) resolveMentions(ctx context.Context, ws pgtype.UUID, body string) ([]Mention, []db.Agent) {
	ms := []Mention{}
	agents := []db.Agent{}
	for _, h := range ParseMentionHandles(body) {
		if a, err := s.Q.GetAgentByHandle(ctx, db.GetAgentByHandleParams{WorkspaceID: ws, Handle: h}); err == nil {
			ms = append(ms, Mention{"agent", util.UUIDString(a.ID), h, a.Kind})
			agents = append(agents, a)
			continue
		}
		if u, err := s.Q.GetUserByHandle(ctx, h); err == nil {
			if _, err := s.Q.GetMemberRole(ctx, db.GetMemberRoleParams{WorkspaceID: ws, UserID: u.ID}); err == nil {
				ms = append(ms, Mention{"user", util.UUIDString(u.ID), h, ""})
			}
		}
	}
	return ms, agents
}

// RoomDTO is a chat room.
type RoomDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	LastSeq   int64  `json:"last_seq"`
}

// ListRooms lists rooms in the workspace.
func (s *Service) ListRooms(ctx context.Context, wsID string) ([]RoomDTO, error) {
	rows, err := s.Q.ListRooms(ctx, util.MustUUID(wsID))
	if err != nil {
		return nil, err
	}
	out := make([]RoomDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, RoomDTO{util.UUIDString(r.ID), util.UUIDString(r.ProjectID), r.Name, r.LastSeq})
	}
	return out, nil
}

func (s *Service) room(ctx context.Context, wsID, roomID string) (db.Room, error) {
	id, err := util.ParseUUID(roomID)
	if err != nil {
		return db.Room{}, apperr.ErrNotFound
	}
	r, err := s.Q.GetRoom(ctx, id)
	if err != nil || util.UUIDString(r.WorkspaceID) != wsID {
		return db.Room{}, apperr.ErrNotFound
	}
	return r, nil
}

// ListMessages returns messages after seq.
func (s *Service) ListMessages(ctx context.Context, wsID, roomID string, after int64) ([]MessageDTO, error) {
	r, err := s.room(ctx, wsID, roomID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListMessages(ctx, db.ListMessagesParams{RoomID: r.ID, Seq: after, Limit: 500})
	if err != nil {
		return nil, err
	}
	out := make([]MessageDTO, 0, len(rows))
	for _, m := range rows {
		out = append(out, messageDTO(m))
	}
	return out, nil
}

type post struct {
	room        db.Room
	senderType  string
	senderID    pgtype.UUID
	clientMsgID string
	body        string
	mentions    []Mention
	card        Card
	trust       string
}

// postMessage allocates the next room seq and inserts atomically.
func (s *Service) postMessage(ctx context.Context, p post) (db.Message, error) {
	if p.trust == "" {
		p.trust = "trusted"
	}
	if p.mentions == nil {
		p.mentions = []Mention{}
	}
	var cardJSON []byte
	if p.card != nil {
		cardJSON = util.JSON(p.card)
	}
	cm := pgtype.Text{String: p.clientMsgID, Valid: p.clientMsgID != ""}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.Message{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.Q.WithTx(tx)
	seq, err := q.NextRoomSeq(ctx, p.room.ID)
	if err != nil {
		return db.Message{}, err
	}
	m, err := q.InsertMessage(ctx, db.InsertMessageParams{RoomID: p.room.ID, Seq: seq, SenderType: p.senderType, SenderID: p.senderID, ClientMsgID: cm, Body: p.body, Mentions: util.JSON(p.mentions), Card: cardJSON, Trust: p.trust})
	if err != nil {
		return db.Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Message{}, err
	}
	s.broadcast(p.room.WorkspaceID, protocol.TypeRoomMessage, messageDTO(m))
	return m, nil
}

func (s *Service) systemCard(ctx context.Context, roomID pgtype.UUID, body string, card Card) (db.Message, error) {
	r, err := s.Q.GetRoom(ctx, roomID)
	if err != nil {
		return db.Message{}, err
	}
	return s.postMessage(ctx, post{room: r, senderType: "system", body: body, card: card})
}

// patchCard merges fields into a message's card and broadcasts the update.
func (s *Service) patchCard(ctx context.Context, msgID pgtype.UUID, fields Card) (db.Message, Card, error) {
	m, err := s.Q.GetMessage(ctx, msgID)
	if err != nil {
		return db.Message{}, nil, err
	}
	c := Card{}
	_ = json.Unmarshal(m.Card, &c)
	for k, v := range fields {
		c[k] = v
	}
	up, err := s.Q.UpdateMessageCard(ctx, db.UpdateMessageCardParams{ID: msgID, Card: util.JSON(c)})
	if err != nil {
		return db.Message{}, nil, err
	}
	r, _ := s.Q.GetRoom(ctx, up.RoomID)
	s.broadcast(r.WorkspaceID, protocol.TypeRoomMessage, messageDTO(up))
	return up, c, nil
}

func cardOf(m db.Message) Card {
	c := Card{}
	_ = json.Unmarshal(m.Card, &c)
	return c
}

// SendInput is a user chat message.
type SendInput struct {
	ClientMsgID string `json:"client_msg_id"`
	Body        string `json:"body"`
}

// SendMessage posts a user message (deduped by client_msg_id) and reacts to
// @mentions: a specialist mention produces a delegation draft card for the
// sender, a mention of the sender's own chief agent starts a chat run.
func (s *Service) SendMessage(ctx context.Context, wsID, userID, roomID string, in SendInput) (MessageDTO, error) {
	r, err := s.room(ctx, wsID, roomID)
	if err != nil {
		return MessageDTO{}, err
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		return MessageDTO{}, apperr.BadRequest("body is required")
	}
	if len(in.Body) > 16000 {
		return MessageDTO{}, apperr.BadRequest("message too long")
	}
	if in.ClientMsgID != "" {
		if m, err := s.Q.GetMessageByClientID(ctx, db.GetMessageByClientIDParams{RoomID: r.ID, ClientMsgID: pgtype.Text{String: in.ClientMsgID, Valid: true}}); err == nil {
			return messageDTO(m), nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return MessageDTO{}, err
		}
	}
	mentions, agents := s.resolveMentions(ctx, r.WorkspaceID, in.Body)
	uid := util.MustUUID(userID)
	m, err := s.postMessage(ctx, post{room: r, senderType: "user", senderID: uid, clientMsgID: in.ClientMsgID, body: in.Body, mentions: mentions})
	if err != nil {
		if m2, err2 := s.Q.GetMessageByClientID(ctx, db.GetMessageByClientIDParams{RoomID: r.ID, ClientMsgID: pgtype.Text{String: in.ClientMsgID, Valid: in.ClientMsgID != ""}}); err2 == nil {
			return messageDTO(m2), nil
		}
		return MessageDTO{}, err
	}
	for _, a := range agents {
		switch a.Kind {
		case "specialist":
			if _, err := s.createDraft(ctx, r, userID, m, a, draftFromText(in.Body)); err != nil {
				s.log.Error("draft failed", "err", err)
			}
		case "chief":
			if util.UUIDString(a.OwnerID) != userID {
				_, _ = s.systemCard(ctx, r.ID, "@"+a.Handle+" 只响应其所有者的指令。", Card{"type": "notice", "code": "unauthorized"})
				continue
			}
			if err := s.startChiefRun(ctx, r, userID, m, a); err != nil {
				_, _ = s.systemCard(ctx, r.ID, "Chief agent 无法启动："+err.Error(), Card{"type": "notice", "code": "failed"})
			}
		}
	}
	return messageDTO(m), nil
}

// Draft is the editable content of a delegation draft card.
type Draft struct {
	Title     string              `json:"title"`
	Prompt    string              `json:"prompt"`
	Grant     protocol.Grant      `json:"grant"`
	Budget    protocol.Budget     `json:"budget"`
	Approvers []protocol.Approver `json:"approvers"`
	MaxDepth  int                 `json:"max_depth"`
}

func draftFromText(body string) Draft {
	t := strings.TrimSpace(mentionRe.ReplaceAllString(body, ""))
	title := strings.SplitN(t, "\n", 2)[0]
	if r := []rune(title); len(r) > 80 {
		title = string(r[:80])
	}
	if title == "" {
		title = "未命名任务"
	}
	return Draft{Title: title, Prompt: t}
}

func (s *Service) withDraftDefaults(ctx context.Context, d Draft, requesterID string) Draft {
	if len(d.Grant.Resources) == 0 {
		d.Grant.Resources = []string{"src/**"}
	}
	if len(d.Grant.Actions) == 0 {
		d.Grant.Actions = []string{"read", "edit", "execute"}
	}
	if d.Grant.Network == nil {
		d.Grant.Network = []string{}
	}
	if d.Budget.MaxTokens == 0 {
		d.Budget.MaxTokens = 3_000_000
	}
	if d.Budget.MaxDurationMin == 0 {
		d.Budget.MaxDurationMin = 30
	}
	if d.MaxDepth == 0 {
		d.MaxDepth = 3
	}
	if len(d.Approvers) == 0 {
		if u, err := s.Q.GetUser(ctx, util.MustUUID(requesterID)); err == nil {
			d.Approvers = []protocol.Approver{{ID: requesterID, Name: u.Name, Role: "requester"}}
		}
	}
	return d
}

func (s *Service) createDraft(ctx context.Context, r db.Room, requesterID string, src db.Message, target db.Agent, d Draft) (db.Message, error) {
	d = s.withDraftDefaults(ctx, d, requesterID)
	card := Card{
		"type": "delegation_draft", "state": "draft", "requester_id": requesterID,
		"target_agent_id": util.UUIDString(target.ID), "target_handle": target.Handle,
		"target_owner_id": util.UUIDString(target.OwnerID), "source_message_id": util.UUIDString(src.ID),
		"draft": d,
	}
	return s.postMessage(ctx, post{room: r, senderType: "system", body: "委托草稿：@" + target.Handle + " · " + d.Title, card: card})
}

// ConfirmDraft turns a draft card into a delegation. The draft message id is
// the idempotency key, so double clicks and retries create one task.
func (s *Service) ConfirmDraft(ctx context.Context, wsID, userID, messageID string, edit *Draft) (TaskDTO, bool, error) {
	id, err := util.ParseUUID(messageID)
	if err != nil {
		return TaskDTO{}, false, apperr.ErrNotFound
	}
	m, err := s.Q.GetMessage(ctx, id)
	if err != nil {
		return TaskDTO{}, false, apperr.ErrNotFound
	}
	r, err := s.room(ctx, wsID, util.UUIDString(m.RoomID))
	if err != nil {
		return TaskDTO{}, false, err
	}
	c := cardOf(m)
	if c["type"] != "delegation_draft" {
		return TaskDTO{}, false, apperr.BadRequest("not a delegation draft")
	}
	if c["requester_id"] != userID {
		return TaskDTO{}, false, apperr.Forbidden("delegation.unauthorized", "only the requester can confirm this draft")
	}
	var d Draft
	b, _ := json.Marshal(c["draft"])
	_ = json.Unmarshal(b, &d)
	if edit != nil {
		d = *edit
	}
	if c["state"] == "discarded" {
		return TaskDTO{}, false, apperr.ErrAlreadyDone
	}
	in := DelegationInput{
		RoomID: util.UUIDString(r.ID), ProjectID: util.UUIDString(r.ProjectID), SourceMessageID: str(c["source_message_id"]),
		TargetAgentID: str(c["target_agent_id"]), IdempotencyKey: "draft:" + messageID, SourceAgentID: str(c["source_agent_id"]),
		Title: d.Title, Prompt: d.Prompt, Grant: d.Grant, Budget: d.Budget, Approvers: d.Approvers, MaxDepth: d.MaxDepth,
		ParentTaskID: str(c["parent_task_id"]),
	}
	t, dup, err := s.CreateDelegation(ctx, wsID, userID, in)
	if err != nil {
		return TaskDTO{}, false, err
	}
	_, _, _ = s.patchCard(ctx, m.ID, Card{"state": "submitted", "task_id": t.ID, "draft": d})
	return t, dup, nil
}

// DiscardDraft marks a draft as discarded.
func (s *Service) DiscardDraft(ctx context.Context, wsID, userID, messageID string) error {
	id, err := util.ParseUUID(messageID)
	if err != nil {
		return apperr.ErrNotFound
	}
	m, err := s.Q.GetMessage(ctx, id)
	if err != nil {
		return apperr.ErrNotFound
	}
	if _, err := s.room(ctx, wsID, util.UUIDString(m.RoomID)); err != nil {
		return err
	}
	c := cardOf(m)
	if c["type"] != "delegation_draft" || c["requester_id"] != userID {
		return apperr.ErrForbidden
	}
	if c["state"] != "draft" {
		return apperr.ErrAlreadyDone
	}
	_, _, err = s.patchCard(ctx, m.ID, Card{"state": "discarded"})
	return err
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
