package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openshift/faas-console-plugin/backend/config"
	"github.com/openshift/faas-console-plugin/backend/scm"
	"github.com/openshift/faas-console-plugin/backend/session"
)

// HandleLogin authenticates a GitHub PAT and creates a session.
// POST /api/v1/auth/login
// Request: { "pat": "ghp_..." }
// Response: 201 { "token": "...", "login": "...", "avatarUrl": "..." }
// The caller sends the token back in the X-FUNC-SESSION header;
// the PAT itself stays on the backend. The session is bound to the OpenShift
// user that created it.
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ocpUser, err := h.currentUser(r)
	if err != nil {
		slog.Warn("login without a resolvable OpenShift user", "err", err)
		writeError(w, http.StatusUnauthorized, "openshift authentication required")
		return
	}

	var req struct {
		PAT string `json:"pat"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PAT == "" {
		writeError(w, http.StatusBadRequest, "pat is required")
		return
	}

	// Verify PAT by fetching the user from GitHub
	scmClient, err := config.SCMRegistry.NewClient(scm.GitHub, req.PAT)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create scm client")
		return
	}
	user, err := scmClient.GetUser(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid github pat")
		return
	}

	token, err := h.sessionStore.CreateSession(r.Context(), ocpUser, user.Login, req.PAT, session.CredentialTypePAT)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"token":     token,
		"login":     user.Login,
		"avatarUrl": user.AvatarURL,
	})
}

func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get(sessionHeader)
	if token == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.sessionStore.DeleteSession(r.Context(), token); err != nil {
		slog.Error("failed to delete session", "err", err)
	}

	w.WriteHeader(http.StatusNoContent)
}
