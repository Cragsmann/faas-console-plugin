package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/faas-console-plugin/backend/config"
	"github.com/openshift/faas-console-plugin/backend/identity"
	"github.com/openshift/faas-console-plugin/backend/scm"
	"github.com/openshift/faas-console-plugin/backend/scm/credentials"
)

var _ = Describe("POST /api/v1/scm/github/credential", func() {
	BeforeEach(func() {
		withSCMStub(&scm.ClientStub{
			OnGetUser: func(ctx context.Context) (*scm.User, error) {
				return &scm.User{Login: "alice-gh", AvatarURL: "https://example.com/avatar"}, nil
			},
		})
	})

	It("keeps each caller's saved GitHub account separate", func() {
		original := config.SCMRegistry
		config.SCMRegistry = scm.Registry{scm.GitHub: func(pat string) scm.Client {
			return &scm.ClientStub{
				OnGetUser: func(context.Context) (*scm.User, error) {
					login := "alice-gh"
					if pat == "bob-pat" {
						login = "bob-gh"
					}
					return &scm.User{Login: login}, nil
				},
				OnGetFiles: func(_ context.Context, owner, _, _ string) ([]scm.FileEntry, error) {
					if pat != owner+"-pat" {
						return nil, scm.ErrUnauthorized
					}
					return []scm.FileEntry{{Path: "func.yaml", Content: "name: " + owner}}, nil
				},
			}
		}}
		DeferCleanup(func() { config.SCMRegistry = original })
		h := &Handlers{scmCredentials: make(credentials.MapProvider)}
		aliceRequest := saveTokenRequest("alice-pat")
		aliceRequest.Header.Set("Authorization", "Bearer alice-ocp")
		bobRequest := saveTokenRequest("bob-pat")
		bobRequest.Header.Set("Authorization", "Bearer bob-ocp")

		aliceResponse := httptest.NewRecorder()
		h.HandleSaveSCMCredential(aliceResponse, aliceRequest)
		Expect(aliceResponse.Code).To(Equal(http.StatusCreated))
		bobResponse := httptest.NewRecorder()
		h.HandleSaveSCMCredential(bobResponse, bobRequest)
		Expect(bobResponse.Code).To(Equal(http.StatusCreated))

		expectAccount(h, "alice-ocp", "alice-gh")
		expectAccount(h, "bob-ocp", "bob-gh")
		for _, owner := range []string{"alice", "bob"} {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/func/"+owner+"/fn/files", nil)
			req.Header.Set("Authorization", "Bearer "+owner+"-ocp")
			req.SetPathValue("owner", owner)
			req.SetPathValue("name", "fn")
			w := httptest.NewRecorder()

			h.HandleGetFiles(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var files []scm.FileEntry
			Expect(json.NewDecoder(w.Body).Decode(&files)).To(Succeed())
			Expect(files).To(Equal([]scm.FileEntry{{Path: "func.yaml", Content: "name: " + owner}}))
		}
	})

	It("returns the GitHub profile, never the token", func() {
		w := httptest.NewRecorder()

		h := &Handlers{scmCredentials: testCredentialProvider()}

		h.HandleSaveSCMCredential(w, saveTokenRequest("ghp_valid"))

		var resp map[string]string
		Expect(json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())
		Expect(resp).To(HaveKeyWithValue("login", "alice-gh"))
		Expect(resp).To(HaveKeyWithValue("avatarUrl", "https://example.com/avatar"))
		Expect(resp).To(HaveLen(2)) // login, avatarUrl and nothing else
	})

	It("returns 400 when no token was sent", func() {
		w := httptest.NewRecorder()

		h := &Handlers{scmCredentials: testCredentialProvider()}

		h.HandleSaveSCMCredential(w, saveTokenRequest(""))

		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	// Not 401: the frontend reads that one status as "the connection is
	// gone" and disconnects, which would throw away a working session over a
	// mistyped PAT. The SCM provider refusing a token the user just supplied
	// is the same class of problem as it refusing a stored one.
	It("returns 403 when the SCM provider rejects the token", func() {
		withSCMStub(&scm.ClientStub{
			OnGetUser: func(ctx context.Context) (*scm.User, error) { return nil, scm.ErrUnauthorized },
		})
		w := httptest.NewRecorder()

		h := &Handlers{scmCredentials: testCredentialProvider()}
		h.HandleSaveSCMCredential(w, saveTokenRequest("ghp_bad"))

		Expect(w.Code).To(Equal(http.StatusForbidden))
		// A token GitHub refuses is never written, so the caller keeps whatever
		// they already had rather than losing it to a typo.
		expectAccount(h, testOCPToken, "tester")
	})

	It("returns 401 when the OpenShift token is rejected", func() {
		h := failingCredentialHandler(fmt.Errorf("%w: token rejected by the API server", identity.ErrUnauthenticated))
		w := httptest.NewRecorder()

		h.HandleSaveSCMCredential(w, saveTokenRequest("ghp_valid"))

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})

	// Not 401: the API server being unreachable says nothing about the caller,
	// and the frontend answers 401 by disconnecting. A blip must not cost the
	// user a connection they still have.
	It("returns 503 when the identity cannot be looked up at all", func() {
		h := failingCredentialHandler(errors.New("dial tcp: connection refused"))
		w := httptest.NewRecorder()

		h.HandleSaveSCMCredential(w, saveTokenRequest("ghp_valid"))

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
	})

	It("returns 401 when the console forwarded no user token", func() {
		h := &Handlers{scmCredentials: testCredentialProvider()}
		req := saveTokenRequest("ghp_valid")
		req.Header.Del("Authorization")
		w := httptest.NewRecorder()

		h.HandleSaveSCMCredential(w, req)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})
})

var _ = Describe("GET /api/v1/scm/github/credential", func() {
	getRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/scm/github/credential", nil)
		req.Header.Set("Authorization", "Bearer "+testOCPToken)
		return req
	}

	// The whole point of keying on identity alone: a browser that has never
	// seen this credential is connected by its OpenShift token and nothing else.
	It("reports the GitHub account behind a stored credential", func() {
		h := &Handlers{scmCredentials: testCredentialProvider()}
		w := httptest.NewRecorder()

		h.HandleGetSCMCredential(w, getRequest())

		Expect(w.Code).To(Equal(http.StatusOK))
		var resp map[string]string
		Expect(json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())
		Expect(resp).To(HaveKeyWithValue("login", "tester"))
		Expect(resp).To(HaveLen(2)) // login, avatarUrl and nothing else
	})

	// The stored token answers requests on the backend; handing it back to the
	// browser would undo the entire reason it lives in a Secret.
	It("never returns the token itself", func() {
		h := &Handlers{scmCredentials: testCredentialProvider()}
		w := httptest.NewRecorder()

		h.HandleGetSCMCredential(w, getRequest())

		Expect(w.Body.String()).NotTo(ContainSubstring("test-pat"))
	})

	// A caller with nothing stored is not an authentication failure: they are
	// known, they have simply never connected. The frontend reads 404 as "show
	// the connect prompt" rather than "something went wrong".
	It("returns 404 when the user has no stored credential", func() {
		h := &Handlers{scmCredentials: make(credentials.MapProvider)}
		w := httptest.NewRecorder()

		h.HandleGetSCMCredential(w, getRequest())

		Expect(w.Code).To(Equal(http.StatusNotFound))
	})

	It("returns 401 when the OpenShift token is rejected", func() {
		h := failingCredentialHandler(fmt.Errorf("%w: token rejected by the API server", identity.ErrUnauthenticated))
		w := httptest.NewRecorder()

		h.HandleGetSCMCredential(w, getRequest())

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})

	// Not 404: the frontend reads that as "there is nothing stored" and shows
	// the connect prompt. Being unable to read the credential is not the same
	// as there not being one.
	It("returns 503 when the identity cannot be looked up at all", func() {
		h := failingCredentialHandler(errors.New("dial tcp: connection refused"))
		w := httptest.NewRecorder()

		h.HandleGetSCMCredential(w, getRequest())

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
	})
})

var _ = Describe("DELETE /api/v1/scm/github/credential", func() {
	DescribeTable("rejects requests without a usable bearer token",
		func(authorization string) {
			h := &Handlers{scmCredentials: testCredentialProvider()}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/scm/github/credential", nil)
			req.Header.Set("Authorization", authorization)
			w := httptest.NewRecorder()

			h.HandleDeleteSCMCredential(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
			expectAccount(h, testOCPToken, "tester")
		},
		Entry("missing header", ""),
		Entry("empty bearer token", "Bearer "),
		Entry("wrong authentication scheme", "Basic credentials"),
	)

	deleteRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/scm/github/credential", nil)
		authenticate(req)
		return req
	}

	It("deletes the credential of the user it belongs to", func() {
		h := &Handlers{scmCredentials: testCredentialProvider()}
		w := httptest.NewRecorder()

		h.HandleDeleteSCMCredential(w, deleteRequest())

		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(credentialResponse(h, testOCPToken).Code).To(Equal(http.StatusNotFound))
	})

	// Revocation follows the identity. Reaching the backend as one user must
	// not be a way to disconnect somebody else's GitHub account.
	It("leaves another user's credential alone", func() {
		provider := testCredentialProvider()
		provider["other-ocp-token"] = credentials.Credential{Owner: "other-user", Token: "other-pat", Type: credentials.CredentialTypePAT}
		h := &Handlers{scmCredentials: provider}
		w := httptest.NewRecorder()

		req := deleteRequest()
		req.Header.Set("Authorization", "Bearer other-ocp-token")
		h.HandleDeleteSCMCredential(w, req)

		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(credentialResponse(h, "other-ocp-token").Code).To(Equal(http.StatusNotFound))
		expectAccount(h, testOCPToken, "tester")
	})

	It("returns 204 when the credential provider is unavailable", func() {
		h := failingCredentialHandler(errors.New("token rejected by the API server"))
		w := httptest.NewRecorder()

		h.HandleDeleteSCMCredential(w, deleteRequest())

		Expect(w.Code).To(Equal(http.StatusNoContent))
	})
})

func failingCredentialHandler(cause error) *Handlers {
	return &Handlers{scmCredentials: failingCredentialProvider(cause)}
}

func credentialResponse(h *Handlers, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/scm/github/credential", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.HandleGetSCMCredential(w, req)
	return w
}

func expectAccount(h *Handlers, token, login string) {
	GinkgoHelper()
	response := credentialResponse(h, token)
	Expect(response.Code).To(Equal(http.StatusOK))
	var account map[string]string
	Expect(json.NewDecoder(response.Body).Decode(&account)).To(Succeed())
	Expect(account["login"]).To(Equal(login))
}

func saveTokenRequest(pat string) *http.Request {
	body, err := json.Marshal(map[string]string{"pat": pat})
	Expect(err).NotTo(HaveOccurred())
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/scm/github/credential", bytes.NewBuffer(body))
	authenticate(req)
	return req
}
