package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/openshift/faas-console-plugin/backend/config"
	"github.com/openshift/faas-console-plugin/backend/scm"
)

func (h *Handlers) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	credential, err := h.extractCredentialFromSession(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	client := config.SCMRegistry.Client(scm.DefaultPlatform, credential)
	user, err := client.GetUser(r.Context())
	if err != nil {
		if errors.Is(err, scm.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "invalid SCM token")
			return
		}
		slog.Error("failed to get SCM user", "err", err)
		writeError(w, http.StatusBadGateway, "failed to reach SCM API")
		return
	}

	writeJSON(w, http.StatusOK, user)
}
