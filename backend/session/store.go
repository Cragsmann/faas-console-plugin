package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Store manages session token to PAT mappings in Kubernetes Secrets.
type Store struct {
	client kubernetes.Interface
}

// secretData holds the data stored in a session Secret.
type secretData struct {
	Token     string    `json:"token"`
	Type      string    `json:"type"`      // "pat" or "oauth"
	Owner     string    `json:"owner"`     // GitHub username or other identifier
	ExpiresAt time.Time `json:"expiresAt"`
}

// NewStore creates a session store using the provided Kubernetes REST config.
func NewStore(cfg *rest.Config) (*Store, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return &Store{client: client}, nil
}

// CreateSession stores a credential in a Kubernetes Secret and returns a session token.
// credentialType should be CredentialTypePAT or CredentialTypeOAuth.
func (s *Store) CreateSession(ctx context.Context, owner, credential, credentialType string) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	data := secretData{
		Token:     credential,
		Type:      credentialType,
		Owner:     owner,
		ExpiresAt: time.Now().Add(SessionTTL),
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal secret data: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(token),
			Namespace: SessionNamespace,
			Labels: map[string]string{
				"app":  "console-functions-plugin",
				"type": "session",
			},
		},
		Data: map[string][]byte{
			"session": dataBytes,
		},
	}

	_, err = s.client.CoreV1().Secrets(SessionNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create secret: %w", err)
	}

	return token, nil
}

// GetCredential retrieves the credential (PAT or OAuth token) and owner associated with a session token.
// Returns the credential, credential type, owner, and error if expired or not found.
func (s *Store) GetCredential(ctx context.Context, token string) (credential, credentialType, owner string, err error) {
	secret, err := s.client.CoreV1().Secrets(SessionNamespace).Get(ctx, SecretName(token), metav1.GetOptions{})
	if err != nil {
		return "", "", "", fmt.Errorf("get secret: %w", err)
	}

	dataBytes, ok := secret.Data["session"]
	if !ok {
		return "", "", "", fmt.Errorf("session data not found in secret")
	}

	var data secretData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		return "", "", "", fmt.Errorf("unmarshal session data: %w", err)
	}

	if time.Now().After(data.ExpiresAt) {
		s.DeleteSession(ctx, token) // best effort cleanup
		return "", "", "", fmt.Errorf("session expired")
	}

	return data.Token, data.Type, data.Owner, nil
}

// GetPAT is a convenience method that retrieves the credential and verifies it's a PAT.
// Deprecated: Use GetCredential for credential-type-agnostic access.
func (s *Store) GetPAT(ctx context.Context, token string) (pat, owner string, err error) {
	credential, credentialType, owner, err := s.GetCredential(ctx, token)
	if err != nil {
		return "", "", err
	}
	if credentialType != CredentialTypePAT {
		return "", "", fmt.Errorf("expected PAT credential, got %s", credentialType)
	}
	return credential, owner, nil
}

// MemoryStore is an in-memory session store for development/testing.
// It is NOT suitable for production as sessions are lost on restart and not shared across replicas.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]secretData
}

// NewMemoryStore creates an in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]secretData),
	}
}

// CreateSession stores a credential in memory and returns a session token.
func (m *MemoryStore) CreateSession(ctx context.Context, owner, credential, credentialType string) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	data := secretData{
		Token:     credential,
		Type:      credentialType,
		Owner:     owner,
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	m.mu.Lock()
	m.sessions[token] = data
	m.mu.Unlock()

	return token, nil
}

// GetCredential retrieves the credential from memory.
func (m *MemoryStore) GetCredential(ctx context.Context, token string) (credential, credentialType, owner string, err error) {
	m.mu.RLock()
	data, ok := m.sessions[token]
	m.mu.RUnlock()

	if !ok {
		return "", "", "", fmt.Errorf("session not found")
	}

	if time.Now().After(data.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
		return "", "", "", fmt.Errorf("session expired")
	}

	return data.Token, data.Type, data.Owner, nil
}

// DeleteSession removes a session from memory.
func (m *MemoryStore) DeleteSession(ctx context.Context, token string) error {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
	return nil
}

// DeleteSession deletes a session Secret.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.client.CoreV1().Secrets(SessionNamespace).Delete(ctx, SecretName(token), metav1.DeleteOptions{})
}
