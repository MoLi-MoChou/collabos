package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
)

// IM kinds for outbound notifications (PRD phase 3, external IM).
var imKinds = map[string]bool{"slack": true, "feishu": true, "dingtalk": true}

// IMInput configures a workspace's outbound IM webhook.
type IMInput struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// SetIM stores the workspace IM webhook (owner only). The URL host must be a
// known IM host (or listed in COLLAB_IM_ALLOW_HOSTS) so the server cannot be
// pointed at arbitrary internal addresses.
func (s *Service) SetIM(ctx context.Context, wsID, userID string, in IMInput) (map[string]string, error) {
	role, _ := s.RequireMember(ctx, wsID, userID)
	if role != "owner" {
		return nil, apperr.Forbidden("im.not_owner", "only the workspace owner can configure IM")
	}
	if in.URL != "" {
		if !imKinds[in.Kind] {
			return nil, apperr.BadRequest("kind must be slack, feishu or dingtalk")
		}
		if err := s.checkIMURL(in.URL); err != nil {
			return nil, err
		}
	} else {
		in.Kind = ""
	}
	w, err := s.Q.SetWorkspaceIM(ctx, db.SetWorkspaceIMParams{ID: util.MustUUID(wsID), ImWebhookUrl: in.URL, ImKind: in.Kind})
	if err != nil {
		return nil, err
	}
	s.Audit(ctx, w.ID, "user", userID, "im.configure", "workspace", wsID, map[string]string{"kind": in.Kind, "host": hostOf(in.URL)})
	return map[string]string{"kind": w.ImKind, "host": hostOf(w.ImWebhookUrl)}, nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (s *Service) checkIMURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return apperr.BadRequest("invalid webhook URL")
	}
	host := u.Hostname()
	for _, h := range s.Cfg.IMAllowHosts {
		if host == h {
			if u.Scheme == "https" || (u.Scheme == "http" && (host == "127.0.0.1" || host == "localhost")) {
				return nil
			}
		}
	}
	return apperr.BadRequest("webhook host is not an allowed IM host")
}

// IMPayload renders a text notification for the IM kind.
func IMPayload(kind, text string) any {
	switch kind {
	case "feishu":
		return map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
	case "dingtalk":
		return map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
	default:
		return map[string]string{"text": text}
	}
}

// notify sends text to the workspace IM webhook, if configured. It never
// blocks the caller and never includes secrets.
func (s *Service) notify(ctx context.Context, ws pgtype.UUID, room pgtype.UUID, text string) {
	w, err := s.Q.GetWorkspace(ctx, ws)
	if err != nil || w.ImWebhookUrl == "" || s.checkIMURL(w.ImWebhookUrl) != nil {
		return
	}
	if r, err := s.Q.GetRoom(ctx, room); err == nil {
		text = "[CollabOS · " + r.Name + "] " + text
	}
	body, _ := json.Marshal(IMPayload(w.ImKind, text))
	go func(u string) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.HTTPClient.Do(req)
		if err != nil {
			s.log.Warn("im notify", "err", strings.ReplaceAll(err.Error(), u, hostOf(u)))
			return
		}
		_ = resp.Body.Close()
	}(w.ImWebhookUrl)
}
