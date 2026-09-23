package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/openshift/faas-console-plugin/backend/identity"
	"github.com/openshift/faas-console-plugin/backend/session"
)

type Handlers struct {
	caCert               []byte            // cluster CA certificate, read once at startup
	kubeHost             string            // API server URL for dev/test; empty uses in-cluster config
	externalAPIServerURL string            // external URL embedded in generated kubeconfigs
	saTokenExpiry        int64             // requested SA token lifetime in seconds
	sessionStore         *session.Store    // session token to PAT mapping
	identityResolver     identity.Resolver // OCP user behind the console's bearer token
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

func New(caPath, kubeHost, externalAPIServerURL string, saTokenExpiry int64, sessionStore *session.Store) (*Handlers, error) {
	var caCert []byte
	if caPath != "" {
		var err error
		caCert, err = os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate %q: %w", caPath, err)
		}
	}
	return &Handlers{
		caCert:               caCert,
		kubeHost:             kubeHost,
		externalAPIServerURL: externalAPIServerURL,
		saTokenExpiry:        saTokenExpiry,
		sessionStore:         sessionStore,
		identityResolver:     identity.NewResolver(kubeHost, caCert),
	}, nil
}

const sessionHeader = "X-FUNC-SESSION"

func (h *Handlers) extractCredentialFromSession(r *http.Request) (string, error) {
	// Deliberately not Authorization: that header carries the OCP user token
	// forwarded by the console proxy (see extractOCPToken).
	token := r.Header.Get(sessionHeader)
	if token == "" {
		return "", fmt.Errorf("no %s header", sessionHeader)
	}

	user, err := h.currentUser(r)
	if err != nil {
		return "", err
	}

	credential, err := h.sessionStore.GetCredential(r.Context(), token, user)
	if err != nil {
		return "", fmt.Errorf("invalid or expired session: %w", err)
	}
	return credential, nil
}

func (h *Handlers) currentUser(r *http.Request) (identity.User, error) {
	ocpToken, ok := extractOCPToken(r)
	if !ok {
		return identity.User{}, fmt.Errorf("no OpenShift user token")
	}

	user, err := h.identityResolver.Resolve(r.Context(), ocpToken)
	if err != nil {
		return identity.User{}, fmt.Errorf("resolve OpenShift user: %w", err)
	}
	return user, nil
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
