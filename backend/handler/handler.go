package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openshift/faas-console-plugin/backend/auth"
)

type Handlers struct {
	caCert               []byte      // cluster CA certificate, read once at startup
	kubeHost             string      // API server URL for dev/test; empty uses in-cluster config
	externalAPIServerURL string      // external URL embedded in generated kubeconfigs
	saTokenExpiry        int64       // requested SA token lifetime in seconds
	credentials          *auth.Store // SCM credentials, resolved from the console's bearer token
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

func New(caCert []byte, kubeHost, externalAPIServerURL string, saTokenExpiry int64, credentials *auth.Store) *Handlers {
	return &Handlers{
		caCert:               caCert,
		kubeHost:             kubeHost,
		externalAPIServerURL: externalAPIServerURL,
		saTokenExpiry:        saTokenExpiry,
		credentials:          credentials,
	}
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
