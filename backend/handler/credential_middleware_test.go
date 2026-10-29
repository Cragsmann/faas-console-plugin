package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/faas-console-plugin/backend/identity"
	"github.com/openshift/faas-console-plugin/backend/scm/credentials"
)

var _ = Describe("credential middleware", func() {
	DescribeTable("rejects missing or malformed bearer authentication",
		func(authorization string) {
			h := &Handlers{scmCredentials: withCredentialProviderError(errors.New("cluster unavailable"))}
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/func/list", nil)
			req.Header.Set("Authorization", authorization)
			w := httptest.NewRecorder()
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

			h.RequireCredential(next).ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			Expect(w.Body.String()).To(MatchJSON(`{"message":"authentication required"}`))
		},
		Entry("missing header", ""),
		Entry("empty bearer token", "Bearer "),
		Entry("wrong authentication scheme", "Basic credentials"),
	)

	It("makes the stored credential and caller token available to the next handler", func() {
		h := &Handlers{scmCredentials: withCredentialProviderStub()}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/func/list", nil)
		authenticate(req)
		w := httptest.NewRecorder()
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			credential := credentialFrom(r.Context())
			writeJSON(w, http.StatusOK, map[string]string{"scmToken": credential.Token, "ocpToken": credential.ocpToken})
		})

		h.RequireCredential(next).ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Body.String()).To(MatchJSON(`{"scmToken":"test-pat","ocpToken":"ocp-token"}`))
	})

	DescribeTable("reports credential lookup failures before entering the next handler",
		func(cause error, code int, path string) {
			h := &Handlers{scmCredentials: withCredentialProviderError(cause)}
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
			authenticate(req)
			w := httptest.NewRecorder()
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

			h.RequireCredential(next).ServeHTTP(w, req)

			Expect(w.Code).To(Equal(code))
		},
		Entry("rejected OpenShift token", identity.ErrUnauthenticated, http.StatusUnauthorized, "/api/v1/func/list"),
		Entry("missing credential", credentials.ErrNoCredential, http.StatusUnauthorized, "/api/v1/func/list"),
		Entry("unavailable store", errors.New("cluster unavailable"), http.StatusServiceUnavailable, "/api/v1/func/list"),
		Entry("unconnected account", credentials.ErrNoCredential, http.StatusNotFound, "/api/v1/scm/github/credential"),
		Entry("account lookup with rejected OpenShift token", identity.ErrUnauthenticated, http.StatusUnauthorized, "/api/v1/scm/github/credential"),
		Entry("account lookup with unavailable store", errors.New("cluster unavailable"), http.StatusServiceUnavailable, "/api/v1/scm/github/credential"),
	)

	It("rejects callers who have no credential even when another caller is connected", func() {
		h := &Handlers{scmCredentials: credentials.MapProvider{"other-token": {Token: "other-pat"}}}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/func/list", nil)
		authenticate(req)
		w := httptest.NewRecorder()
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

		h.RequireCredential(next).ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})
})
