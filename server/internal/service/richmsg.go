package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// RichComponents is the OpenUI component whitelist (PRD 7.1.5): form,
// table, chart, checklist, choices, code block and callout, plus layout and
// text primitives. Platform cards (approval, task, delegation, diff) are not
// in the list: only the server can create them.
var RichComponents = map[string]bool{
	"Stack": true, "Text": true, "Callout": true, "CodeBlock": true, "Table": true,
	"Chart": true, "Checklist": true, "Choices": true, "Form": true, "Field": true, "Button": true,
}

const maxRichSource = 16 << 10

var (
	callRe      = regexp.MustCompile(`\b([A-Z][A-Za-z0-9]*)\s*\(`)
	forbiddenRe = regexp.MustCompile(`(?i)<\s*(script|iframe|object|embed|img|style|link)|javascript:|data:text/html|https?://`)
	openuiFence = regexp.MustCompile("(?s)```openui\\s*\\n(.*?)```")
)

// ValidateRich checks a rich message: OpenUI Lang only, whitelisted
// components, no HTML/iframes, no direct external URLs, bounded size.
func ValidateRich(ui protocol.RichUI) error {
	if ui.Lang != "openui" {
		return fmt.Errorf("unsupported rich lang %q", ui.Lang)
	}
	if len(ui.Source) == 0 || len(ui.Source) > maxRichSource {
		return fmt.Errorf("rich source size out of bounds")
	}
	if forbiddenRe.MatchString(ui.Source) {
		return fmt.Errorf("rich source contains HTML, scripts or external URLs")
	}
	src := stripStrings(ui.Source)
	for _, m := range callRe.FindAllStringSubmatch(src, -1) {
		if !RichComponents[m[1]] {
			return fmt.Errorf("component %s is not whitelisted", m[1])
		}
	}
	return nil
}

// stripStrings blanks string literals so their contents are not mistaken for calls.
func stripStrings(s string) string {
	var b strings.Builder
	in, esc := byte(0), false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if in != 0 {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == in {
				in = 0
			}
			b.WriteByte(' ')
			continue
		}
		if c == '"' || c == '\'' {
			in = c
		}
		b.WriteByte(c)
	}
	return b.String()
}

// ExtractRich returns the rich UI carried in _meta, or the first ```openui
// fence in the text (fallback for agents that cannot set _meta), and the text
// with the fence removed as the plain-text degradation.
func ExtractRich(meta *protocol.RichUI, text string) (*protocol.RichUI, string) {
	if m := openuiFence.FindStringSubmatchIndex(text); m != nil {
		src := strings.TrimSpace(text[m[2]:m[3]])
		rest := strings.TrimSpace(text[:m[0]] + text[m[1]:])
		if meta == nil {
			meta = &protocol.RichUI{Lang: "openui", Version: "1.0", Source: src}
		}
		return meta, rest
	}
	return meta, text
}

// RichMeta is stored with a rich message: content plus its provenance.
type RichMeta struct {
	protocol.RichUI
	AgentID string `json:"agent_id"`
	OwnerID string `json:"owner_id"`
	Trust   string `json:"trust"`
}

// attachRich validates and stores a rich payload on an agent message. An
// invalid payload is dropped (the plain text stays) and audited.
func (s *Service) attachRich(ctx context.Context, m db.Message, agent db.Agent, ui *protocol.RichUI) db.Message {
	if ui == nil {
		return m
	}
	if ui.Version == "" {
		ui.Version = "1.0"
	}
	r, _ := s.Q.GetRoom(ctx, m.RoomID)
	if err := ValidateRich(*ui); err != nil {
		s.Audit(ctx, r.WorkspaceID, "agent", util.UUIDString(agent.ID), "richmsg.rejected", "message", util.UUIDString(m.ID), map[string]string{"error": err.Error()})
		return m
	}
	meta := RichMeta{RichUI: *ui, AgentID: util.UUIDString(agent.ID), OwnerID: util.UUIDString(agent.OwnerID), Trust: "untrusted"}
	up, err := s.Q.SetMessageRich(ctx, db.SetMessageRichParams{ID: m.ID, Rich: util.JSON(meta)})
	if err != nil {
		return m
	}
	s.broadcast(r.WorkspaceID, protocol.TypeRoomMessage, messageDTO(up))
	return up
}

// RichActionInput is a button press or form submit on a rich message.
type RichActionInput struct {
	ActionID string          `json:"action_id"`
	Value    json.RawMessage `json:"value"`
}

var actionIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// RichAction records an interaction under the clicker's identity and relays
// it back as user_input. It can never approve, merge, delegate or spend:
// anything like that needs the platform's own cards (PRD 7.1.5, 11.4).
func (s *Service) RichAction(ctx context.Context, wsID, userID, messageID string, in RichActionInput) (MessageDTO, error) {
	id, err := util.ParseUUID(messageID)
	if err != nil {
		return MessageDTO{}, apperr.ErrNotFound
	}
	m, err := s.Q.GetMessage(ctx, id)
	if err != nil || len(m.Rich) == 0 {
		return MessageDTO{}, apperr.ErrNotFound
	}
	r, err := s.room(ctx, wsID, util.UUIDString(m.RoomID))
	if err != nil {
		return MessageDTO{}, err
	}
	if err := s.requireProject(ctx, r, userID, true); err != nil {
		return MessageDTO{}, err
	}
	if !actionIDRe.MatchString(in.ActionID) {
		return MessageDTO{}, apperr.BadRequest("invalid action_id")
	}
	if len(in.Value) > 4096 {
		return MessageDTO{}, apperr.BadRequest("value too large")
	}
	if len(in.Value) == 0 {
		in.Value = json.RawMessage("{}")
	}
	if _, err := s.Q.InsertRichAction(ctx, db.InsertRichActionParams{MessageID: m.ID, ActionID: in.ActionID, UserID: util.MustUUID(userID), Value: in.Value}); err != nil {
		return MessageDTO{}, apperr.ErrAlreadyDone
	}
	var meta RichMeta
	_ = json.Unmarshal(m.Rich, &meta)
	s.Audit(ctx, r.WorkspaceID, "user", userID, "richmsg.action", "message", messageID, map[string]any{"action_id": in.ActionID, "agent_id": meta.AgentID})
	body := fmt.Sprintf("[富消息交互] %s：%s", in.ActionID, compactJSON(in.Value))
	reply, err := s.postMessage(ctx, post{room: r, senderType: "user", senderID: util.MustUUID(userID), body: body, trust: "user_input",
		card: Card{"type": "rich_reply", "message_id": messageID, "action_id": in.ActionID}})
	if err != nil {
		return MessageDTO{}, err
	}
	// Relay to the bot only when the clicker owns it (a chief answers only its owner).
	if a, err := s.Q.GetAgent(ctx, util.MustUUID(meta.AgentID)); err == nil && a.Kind == "chief" && util.UUIDString(a.OwnerID) == userID {
		if err := s.startChiefRun(ctx, r, userID, reply, a); err != nil {
			s.log.Warn("rich relay", "err", err)
		}
	}
	return messageDTO(reply), nil
}

func compactJSON(b json.RawMessage) string {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	out, _ := json.Marshal(v)
	if len(out) > 400 {
		return string(out[:400]) + "…"
	}
	return string(out)
}
