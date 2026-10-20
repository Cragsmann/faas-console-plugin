package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openshift/faas-console-plugin/backend/auth"
	"github.com/openshift/faas-console-plugin/backend/config"
	"github.com/openshift/faas-console-plugin/backend/scm"
)

func extractOCPToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	return token, token != ""
}

// writeAuthError is the only source of 401 on the request path, which is what
// lets the frontend read that one status as "connect to GitHub".
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated),
		errors.Is(err, auth.ErrNoCredential):
		slog.Warn("rejecting request without a usable credential", "err", err)
		writeError(w, http.StatusUnauthorized, "authentication required")
	default:
		slog.Error("could not resolve the caller's credential", "err", err)
		writeError(w, http.StatusServiceUnavailable, "credential store unavailable")
	}
}

func (h *Handlers) HandleSaveGitHubToken(w http.ResponseWriter, r *http.Request) {
	ocpToken, _ := extractOCPToken(r)

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

	// Verify the token by fetching the user from GitHub, so a credential that
	// cannot work is never written over one that still does.
	scmClient, err := config.SCMRegistry.NewClient(scm.DefaultPlatform, req.PAT)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create scm client")
		return
	}
	user, err := scmClient.GetUser(r.Context())
	if err != nil {
		if errors.Is(err, scm.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "invalid github pat")
			return
		}
		slog.Error("failed to verify pat with the scm provider", "err", err)
		writeError(w, http.StatusBadGateway, "failed to reach the SCM API")
		return
	}

	err = h.credentials.SaveCredential(r.Context(), ocpToken, auth.Credential{
		Owner:     user.Login,
		AvatarURL: user.AvatarURL,
		Secret:    req.PAT,
		Type:      auth.CredentialTypePAT,
	})
	if err != nil {
		// writeAuthError splits these: a refused identity is the caller's
		// problem, anything else is the store being unreachable.
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, githubAccount(user.Login, user.AvatarURL))
}

func (h *Handlers) HandleGetGitHubUser(w http.ResponseWriter, r *http.Request) {
	ocpToken, _ := extractOCPToken(r)

	credential, err := h.credentials.GetCredential(r.Context(), ocpToken)
	if errors.Is(err, auth.ErrNoCredential) {
		writeError(w, http.StatusNotFound, "no stored credential")
		return
	}
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			writeAuthError(w, err)
			return
		}
		slog.Error("failed to read the stored credential", "err", err)
		writeError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	writeJSON(w, http.StatusOK, githubAccount(credential.Owner, credential.AvatarURL))
}

func (h *Handlers) HandleDeleteGitHubToken(w http.ResponseWriter, r *http.Request) {
	ocpToken, _ := extractOCPToken(r)

	// Nothing to revoke if the caller cannot be identified. The browser
	// disconnects regardless, so this is not worth failing the request over.
	if err := h.credentials.DeleteCredential(r.Context(), ocpToken); err != nil {
		slog.Error("failed to delete the stored credential", "err", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func githubAccount(login, avatarURL string) map[string]string {
	return map[string]string{"login": login, "avatarUrl": avatarURL}
}
