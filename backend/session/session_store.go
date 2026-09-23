// Package session keeps SCM credentials in Kubernetes Secrets instead of the
// browser. A caller trades a session token plus its OpenShift identity for the
// credential; the credential itself never leaves the backend.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/openshift/faas-console-plugin/backend/identity"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// sessionTTL is the lifetime of a session token.
	sessionTTL = 24 * time.Hour
	// tokenLength is the byte length of generated tokens.
	tokenLength = 16

	// Credential types recorded in the Secret, so a reader can tell what kind of
	// credential it holds without trying it.
	CredentialTypePAT   = "pat"
	CredentialTypeOAuth = "oauth"
)

// Store keeps session credentials as Secrets.
type Store struct {
	client kubernetes.Interface
	// namespace holds the session Secrets. It must be the namespace the backend
	// runs in, since that is the only one its Role grants Secret access to.
	namespace string
}

// secretData is the JSON blob stored under the Secret's "session" key.
type secretData struct {
	Token string `json:"token"`
	Type  string `json:"type"`  // CredentialTypePAT or CredentialTypeOAuth
	Owner string `json:"owner"` // GitHub username or other identifier
	// User is the OpenShift user the session belongs to. Only that user can
	// trade the session token back for the credential.
	User      identity.User `json:"user"`
	ExpiresAt time.Time     `json:"expiresAt"`
}

// NewStore creates a session store that keeps its Secrets in namespace.
func NewStore(cfg *rest.Config, namespace string) (*Store, error) {
	if namespace == "" {
		return nil, fmt.Errorf("session namespace is required")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return &Store{client: client, namespace: namespace}, nil
}

// NewStoreWithClient builds a Store over an existing clientset. Tests pair it
// with k8s.io/client-go/kubernetes/fake so they exercise the real Secret round
// trip rather than a hand-written stand-in for it.
func NewStoreWithClient(client kubernetes.Interface, namespace string) *Store {
	return &Store{client: client, namespace: namespace}
}

// CreateSession stores a credential in a Secret and returns a session token
// bound to user. credentialType is CredentialTypePAT or CredentialTypeOAuth.
func (s *Store) CreateSession(ctx context.Context, user identity.User, owner, credential, credentialType string) (string, error) {
	if user.Username == "" {
		return "", fmt.Errorf("session requires an OpenShift user")
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}

	dataBytes, err := json.Marshal(secretData{
		Token:     credential,
		Type:      credentialType,
		Owner:     owner,
		User:      user,
		ExpiresAt: time.Now().Add(sessionTTL),
	})
	if err != nil {
		return "", fmt.Errorf("marshal secret data: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName(token),
			Namespace: s.namespace,
			Labels: map[string]string{
				"app":  "console-functions-plugin",
				"type": "session",
			},
		},
		Data: map[string][]byte{
			"session": dataBytes,
		},
	}

	if _, err := s.client.CoreV1().Secrets(s.namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("create secret: %w", err)
	}

	return token, nil
}

// GetCredential returns the credential the session holds. It fails if the token
// is unknown, if the session has expired, or if it belongs to an OpenShift user
// other than user.
func (s *Store) GetCredential(ctx context.Context, token string, user identity.User) (string, error) {
	secret, err := s.client.CoreV1().Secrets(s.namespace).Get(ctx, secretName(token), metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get secret: %w", err)
	}

	dataBytes, ok := secret.Data["session"]
	if !ok {
		return "", fmt.Errorf("session data not found in secret")
	}

	var data secretData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		return "", fmt.Errorf("unmarshal session data: %w", err)
	}

	// Checked before expiry so a token presented by the wrong user is rejected
	// without touching the real owner's session.
	if !data.User.Matches(user) {
		slog.Warn("session token presented by a different OpenShift user",
			"sessionUser", data.User.Username, "caller", user.Username)
		return "", fmt.Errorf("session belongs to another user")
	}

	if time.Now().After(data.ExpiresAt) {
		// Best effort cleanup: the caller is rejected either way.
		if err := s.DeleteSession(ctx, token); err != nil {
			slog.Error("failed to delete expired session", "err", err)
		}
		return "", fmt.Errorf("session expired")
	}

	return data.Token, nil
}

// DeleteSession deletes a session Secret.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if err := s.client.CoreV1().Secrets(s.namespace).Delete(ctx, secretName(token), metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete session secret: %w", err)
	}
	return nil
}

// generateToken creates a cryptographically secure session token.
func generateToken() (string, error) {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// secretName returns the Kubernetes Secret name for a token.
func secretName(token string) string {
	return fmt.Sprintf("ghpat-%s", token)
}
