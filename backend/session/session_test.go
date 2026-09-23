package session_test

import (
	"testing"

	"github.com/openshift/faas-console-plugin/backend/session"
)

func TestGenerateToken(t *testing.T) {
	token1, err := session.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	token2, err := session.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token1 == token2 {
		t.Errorf("tokens should be unique, got %s twice", token1)
	}

	// Token should be 32 hex chars (16 bytes * 2)
	if len(token1) != 32 {
		t.Errorf("token should be 32 chars, got %d", len(token1))
	}
}

func TestSecretName(t *testing.T) {
	token := "abc123"
	secretName := session.SecretName(token)
	expected := "ghpat-abc123"
	if secretName != expected {
		t.Errorf("expected %q, got %q", expected, secretName)
	}
}

func TestConstants(t *testing.T) {
	if session.CredentialTypePAT != "pat" {
		t.Errorf("CredentialTypePAT should be 'pat', got %q", session.CredentialTypePAT)
	}
	if session.CredentialTypeOAuth != "oauth" {
		t.Errorf("CredentialTypeOAuth should be 'oauth', got %q", session.CredentialTypeOAuth)
	}
}
