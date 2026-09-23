package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/faas-console-plugin/backend/identity"
	"github.com/openshift/faas-console-plugin/backend/scm"
)

// mallory is a second OpenShift user holding a session token that is not theirs.
var mallory = identity.User{Username: "mallory", UID: "mallory-uid"}

// resolvingTo builds handlers whose requests authenticate as user.
func resolvingTo(user identity.User) *Handlers {
	return testHandlers(Handlers{identityResolver: &identity.ResolverStub{
		OnResolve: func(context.Context, string) (identity.User, error) { return user, nil },
	}})
}

func loginRequest(pat string) *http.Request {
	body, err := json.Marshal(map[string]string{"pat": pat})
	Expect(err).NotTo(HaveOccurred())
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	authenticate(req)
	// Logging in is how a session is obtained, so there is none to send yet.
	req.Header.Del(sessionHeader)
	return req
}

var _ = Describe("POST /api/v1/auth/login", func() {
	BeforeEach(func() {
		withSCMStub(&scm.ClientStub{
			OnGetUser: func(ctx context.Context) (*scm.User, error) {
				return &scm.User{Login: "alice-gh", AvatarURL: "https://example.com/avatar"}, nil
			},
		})
	})

	// Asserted through the store rather than on what the handler passed it: the
	// token it just minted must work for its owner and for nobody else.
	It("binds the session to the OpenShift user that created it", func() {
		h := resolvingTo(testOCPUser)
		w := httptest.NewRecorder()

		h.HandleLogin(w, loginRequest("ghp_valid"))

		Expect(w.Code).To(Equal(http.StatusCreated))
		var resp map[string]string
		Expect(json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())

		credential, err := h.sessionStore.GetCredential(context.Background(), resp["token"], testOCPUser)
		Expect(err).NotTo(HaveOccurred())
		Expect(credential).To(Equal("ghp_valid"))

		_, err = h.sessionStore.GetCredential(context.Background(), resp["token"], mallory)
		Expect(err).To(HaveOccurred())
	})

	It("returns the session token and the GitHub profile, never the PAT", func() {
		w := httptest.NewRecorder()

		resolvingTo(testOCPUser).HandleLogin(w, loginRequest("ghp_valid"))

		var resp map[string]string
		Expect(json.NewDecoder(w.Body).Decode(&resp)).To(Succeed())
		Expect(resp).To(HaveKeyWithValue("login", "alice-gh"))
		Expect(resp).To(HaveKeyWithValue("avatarUrl", "https://example.com/avatar"))
		Expect(resp).To(HaveLen(3)) // token, login, avatarUrl and nothing else
		Expect(resp["token"]).NotTo(BeEmpty())
		Expect(resp["token"]).NotTo(Equal("ghp_valid"))
	})

	It("returns 401 when the OpenShift user cannot be resolved", func() {
		h := testHandlers(Handlers{identityResolver: &identity.ResolverStub{
			OnResolve: func(context.Context, string) (identity.User, error) {
				return identity.User{}, errors.New("token rejected by the API server")
			},
		}})
		w := httptest.NewRecorder()

		h.HandleLogin(w, loginRequest("ghp_valid"))

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		// Nothing is stored for a caller with no identity to bind to, so the
		// cluster still holds only the session the test started with.
		Expect(sessionSecrets()).To(HaveLen(1))
	})

	It("returns 401 when the console forwarded no user token", func() {
		h := resolvingTo(testOCPUser)
		req := loginRequest("ghp_valid")
		req.Header.Del("Authorization")
		w := httptest.NewRecorder()

		h.HandleLogin(w, req)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
	})
})

var _ = Describe("POST /api/v1/auth/logout", func() {
	logoutRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		authenticate(req)
		return req
	}

	It("deletes the session of the user it belongs to", func() {
		h := resolvingTo(testOCPUser)
		w := httptest.NewRecorder()

		h.HandleLogout(w, logoutRequest())

		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(sessionSecrets()).To(BeEmpty())
	})

	// Logout is the one route the identity does not gate. Presenting the token
	// only ever destroys the session it names, so the worst a leaked token buys
	// is a revocation its holder could trigger by waiting for the TTL anyway.
	It("deletes the session on the token alone, whoever presents it", func() {
		h := resolvingTo(mallory)
		w := httptest.NewRecorder()

		h.HandleLogout(w, logoutRequest())

		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(sessionSecrets()).To(BeEmpty())
	})

	It("returns 204 without touching the store when there is no session header", func() {
		h := resolvingTo(testOCPUser)
		req := logoutRequest()
		req.Header.Del(sessionHeader)
		w := httptest.NewRecorder()

		h.HandleLogout(w, req)

		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(sessionSecrets()).To(HaveLen(1))
	})
})
