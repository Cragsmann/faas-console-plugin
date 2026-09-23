package handler

import (
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
// the PAT itself stays on the backend.
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		PAT string `json:"pat"`
	}
	if err := decodeJSON(r, &req); err != nil {
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

	// Create session in cluster using GitHub username as owner
	token, err := h.sessionStore.CreateSession(r.Context(), user.Login, req.PAT, session.CredentialTypePAT)
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

// HandleLogout revokes the session, deleting the stored credential.
// POST /api/v1/auth/logout
// Response: 204. Unknown or already-deleted sessions also return 204 so the
// caller can clear its local state without special-casing the response.
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
