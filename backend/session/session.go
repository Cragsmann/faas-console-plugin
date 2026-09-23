package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// SessionNamespace is the Kubernetes namespace where session secrets are stored
	SessionNamespace = "openshift-faas-console-plugin"
	// SessionTTL is the lifetime of a session token
	SessionTTL = 24 * time.Hour
	// TokenLength is the byte length of generated tokens
	TokenLength = 16

	// Credential types
	CredentialTypePAT   = "pat"
	CredentialTypeOAuth = "oauth"
)

// SessionStore defines the interface for storing and retrieving session credentials.
type SessionStore interface {
	CreateSession(ctx context.Context, owner, credential, credentialType string) (string, error)
	GetCredential(ctx context.Context, token string) (credential, credentialType, owner string, err error)
	DeleteSession(ctx context.Context, token string) error
}

// GenerateToken creates a cryptographically secure session token.
func GenerateToken() (string, error) {
	b := make([]byte, TokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SecretName returns the Kubernetes Secret name for a token.
func SecretName(token string) string {
	return fmt.Sprintf("ghpat-%s", token)
}
