package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/openshift/faas-console-plugin/backend/session"
)

type Handlers struct {
	caCert               []byte               // cluster CA certificate, read once at startup
	kubeHost             string               // API server URL for dev/test; empty uses in-cluster config
	externalAPIServerURL string               // external URL embedded in generated kubeconfigs
	saTokenExpiry        int64                // requested SA token lifetime in seconds
	sessionStore         session.SessionStore // session token to PAT mapping
}

type httpError struct {
	code    int
	message string
	cause   error
}

func (e *httpError) Error() string {
	return e.message
}

func (e *httpError) Unwrap() error {
	return e.cause
}

func newHTTPError(code int, message string, cause error) error {
	return &httpError{code: code, message: message, cause: cause}
}

func New(caPath, kubeHost, externalAPIServerURL string, saTokenExpiry int64) (*Handlers, error) {
	var caCert []byte
	if caPath != "" {
		var err error
		caCert, err = os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate %q: %w", caPath, err)
		}
	}

	return &Handlers{caCert: caCert, kubeHost: kubeHost, externalAPIServerURL: externalAPIServerURL, saTokenExpiry: saTokenExpiry}, nil
}

func (h *Handlers) SetSessionStore(store session.SessionStore) {
	h.sessionStore = store
}

// sessionHeader carries the session token issued by HandleLogin.
const sessionHeader = "X-FUNC-SESSION"

func extractSCMToken(r *http.Request) (string, bool) {
	v := r.Header.Get("X-SCM-Token")
	return v, v != ""
}

// extractCredentialFromSession retrieves the stored credential (PAT or OAuth token) from the session.
// Falls back to X-SCM-Token header for backward compatibility during migration.
func (h *Handlers) extractCredentialFromSession(r *http.Request) (string, error) {
	// Deliberately not Authorization: that header carries the OCP user token
	// forwarded by the console proxy (see extractOCPToken).
	if token := r.Header.Get(sessionHeader); token != "" {
		credential, _, _, err := h.sessionStore.GetCredential(r.Context(), token)
		if err != nil {
			return "", fmt.Errorf("invalid or expired session: %w", err)
		}
		return credential, nil
	}

	// Fallback to old header for backward compatibility during migration
	pat, ok := extractSCMToken(r)
	if !ok {
		return "", fmt.Errorf("no session token or X-SCM-Token header")
	}
	return pat, nil
}

func extractOCPToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return token, token != ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
