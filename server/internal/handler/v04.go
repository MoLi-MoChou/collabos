package handler

import (
	_ "embed"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/service"
)

//go:embed openapi.yaml
var openapiSpec []byte

// apiV1 serves every /api route also under the versioned /api/v1 prefix (open API, PRD 9.2).
func apiV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			r.URL.Path = "/api/" + strings.TrimPrefix(r.URL.Path, "/api/v1/")
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) openapi(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openapiSpec)
}

func bind[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var in T
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return in, false
	}
	return in, true
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	in, ok := bind[struct {
		RefreshToken string `json:"refresh_token"`
	}](w, r)
	if !ok {
		return
	}
	v, err := h.S.Refresh(r.Context(), in.RefreshToken)
	respond(w, v, err)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	in, ok := bind[struct {
		RefreshToken string `json:"refresh_token"`
	}](w, r)
	if !ok {
		return
	}
	h.S.Logout(r.Context(), bearer(r), in.RefreshToken)
	respond(w, map[string]bool{"ok": true}, nil)
}

func (h *Handler) githubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil {
		writeErr(w, apperr.BadRequest("body too large"))
		return
	}
	v, err := h.S.GitHubWebhook(r.Context(), chi.URLParam(r, "id"), r.Header.Get("X-GitHub-Event"), r.Header.Get("X-GitHub-Delivery"), r.Header.Get("X-Hub-Signature-256"), body)
	respond(w, v, err)
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	_, u := ids(r)
	v, err := h.S.ListAPITokens(r.Context(), u)
	respond(w, v, err)
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	_, u := ids(r)
	in, ok := bind[struct {
		Label string `json:"label"`
	}](w, r)
	if !ok {
		return
	}
	v, err := h.S.CreateAPIToken(r.Context(), u, in.Label)
	respond(w, v, err)
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	_, u := ids(r)
	err := h.S.RevokeAPIToken(r.Context(), u, chi.URLParam(r, "id"))
	respond(w, map[string]bool{"ok": true}, err)
}

func (h *Handler) projectSettings(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.ProjectSettingsInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.UpdateProjectSettings(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) listProjectMembers(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	v, err := h.S.ListProjectMembers(r.Context(), ws, u, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) setProjectMember(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.ProjectMemberInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.SetProjectMember(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) removeProjectMember(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	v, err := h.S.RemoveProjectMember(r.Context(), ws, u, chi.URLParam(r, "id"), chi.URLParam(r, "uid"))
	respond(w, v, err)
}

func (h *Handler) editMemory(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.MemoryEditInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.EditMemory(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) taintMemory(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.TaintInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.TaintMemory(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) memoryRefs(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.MemoryReferences(r.Context(), ws, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) richAction(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.RichActionInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.RichAction(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) postToTask(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.PostToTaskInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.ConvertPostToTask(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) agentPolicy(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.AgentPolicyInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.UpdateAgentPolicy(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) patrol(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.PatrolInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.StartPatrol(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) revokeDaemon(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	err := h.S.RevokeDaemon(r.Context(), ws, u, chi.URLParam(r, "id"))
	respond(w, map[string]bool{"ok": true}, err)
}

func (h *Handler) nudge(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	v, err := h.S.Nudge(r.Context(), ws, u, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) expandBudget(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.BudgetExpansion](w, r)
	if !ok {
		return
	}
	v, err := h.S.ExpandTaskBudget(r.Context(), ws, u, chi.URLParam(r, "id"), in)
	respond(w, v, err)
}

func (h *Handler) trace(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.GetTrace(r.Context(), ws, chi.URLParam(r, "id"))
	respond(w, v, err)
}

func (h *Handler) usage(w http.ResponseWriter, r *http.Request) {
	ws, _ := ids(r)
	v, err := h.S.Usage(r.Context(), ws)
	respond(w, v, err)
}

func (h *Handler) setBudget(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.BudgetInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.SetBudget(r.Context(), ws, u, in)
	respond(w, v, err)
}

func (h *Handler) setIM(w http.ResponseWriter, r *http.Request) {
	ws, u := ids(r)
	in, ok := bind[service.IMInput](w, r)
	if !ok {
		return
	}
	v, err := h.S.SetIM(r.Context(), ws, u, in)
	respond(w, v, err)
}
