package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift/faas-console-plugin/backend/identity"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "console-functions-plugin"

// alice owns every session these tests create; mallory holds a stolen token.
var (
	alice   = identity.User{Username: "alice", UID: "alice-uid"}
	mallory = identity.User{Username: "mallory", UID: "mallory-uid"}
)

func newTestStore(objects ...runtime.Object) *Store {
	return NewStoreWithClient(fake.NewClientset(objects...), testNamespace)
}

func TestCreateSessionStoresSecretInConfiguredNamespace(t *testing.T) {
	store := newTestStore()

	token, err := store.CreateSession(context.Background(), alice, "alice-gh", "ghp_secret", CredentialTypePAT)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	secret, err := store.client.CoreV1().Secrets(testNamespace).Get(context.Background(), secretName(token), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("session secret not found in %q: %v", testNamespace, err)
	}
	if secret.Namespace != testNamespace {
		t.Errorf("secret namespace should be %q, got %q", testNamespace, secret.Namespace)
	}
}

// The credential type and owner are written but never read back, so only a test
// against the stored bytes keeps them honest. They exist so that whoever reads a
// Secret can tell what it holds and whose account it is without trying it.
func TestCreateSessionRecordsTheCredentialTypeAndOwner(t *testing.T) {
	store := newTestStore()

	token, err := store.CreateSession(context.Background(), alice, "alice-gh", "gho_secret", CredentialTypeOAuth)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	data := readSecretData(t, store, token)
	if data.Type != CredentialTypeOAuth {
		t.Errorf("credential type should be %q, got %q", CredentialTypeOAuth, data.Type)
	}
	if data.Owner != "alice-gh" {
		t.Errorf("owner should be %q, got %q", "alice-gh", data.Owner)
	}
	if data.User != alice {
		t.Errorf("session should be bound to %+v, got %+v", alice, data.User)
	}
}

func TestCreateSessionRequiresAnOpenShiftUser(t *testing.T) {
	store := newTestStore()

	if _, err := store.CreateSession(context.Background(), identity.User{}, "alice-gh", "ghp_secret", CredentialTypePAT); err == nil {
		t.Error("CreateSession should reject a session with no OpenShift user to bind to")
	}
}

func TestGetCredentialReturnsStoredCredential(t *testing.T) {
	store := newTestStore()

	token, err := store.CreateSession(context.Background(), alice, "alice-gh", "ghp_secret", CredentialTypePAT)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	credential, err := store.GetCredential(context.Background(), token, alice)
	if err != nil {
		t.Fatalf("GetCredential failed: %v", err)
	}
	if credential != "ghp_secret" {
		t.Errorf("credential should be %q, got %q", "ghp_secret", credential)
	}
}

func TestGetCredentialRejectsAnotherOpenShiftUser(t *testing.T) {
	store := newTestStore()

	token, err := store.CreateSession(context.Background(), alice, "alice-gh", "ghp_secret", CredentialTypePAT)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if _, err := store.GetCredential(context.Background(), token, mallory); err == nil {
		t.Fatal("GetCredential should reject a token presented by a different OpenShift user")
	}

	// The rightful owner keeps the session: a stolen token must not be a way to
	// revoke somebody else's credential.
	if _, err := store.GetCredential(context.Background(), token, alice); err != nil {
		t.Errorf("owner should still reach the session, got %v", err)
	}
}

func TestGetCredentialRejectsUnknownToken(t *testing.T) {
	store := newTestStore()

	if _, err := store.GetCredential(context.Background(), "nosuchtoken", alice); err == nil {
		t.Error("GetCredential should fail for an unknown token")
	}
}

func TestGetCredentialRejectsAndDeletesExpiredSession(t *testing.T) {
	const token = "expiredtoken"
	store := newTestStore(expiredSecret(token))

	if _, err := store.GetCredential(context.Background(), token, alice); err == nil {
		t.Fatal("GetCredential should fail for an expired session")
	}

	_, err := store.client.CoreV1().Secrets(testNamespace).Get(context.Background(), secretName(token), metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("expired session secret should be deleted, got err %v", err)
	}
}

func TestDeleteSessionRemovesSecret(t *testing.T) {
	store := newTestStore()

	token, err := store.CreateSession(context.Background(), alice, "alice-gh", "ghp_secret", CredentialTypePAT)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if err := store.DeleteSession(context.Background(), token); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	if _, err := store.GetCredential(context.Background(), token, alice); err == nil {
		t.Error("GetCredential should fail after the session is deleted")
	}
}

func TestNewStoreRequiresNamespace(t *testing.T) {
	if _, err := NewStore(nil, ""); err == nil {
		t.Error("NewStore should reject an empty namespace")
	}
}

func TestGenerateTokenIsUniqueAndFullLength(t *testing.T) {
	token1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}
	token2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	if token1 == token2 {
		t.Errorf("tokens should be unique, got %s twice", token1)
	}
	// Hex doubles the byte count, and a short token is a guessable token.
	if len(token1) != tokenLength*2 {
		t.Errorf("token should be %d chars, got %d", tokenLength*2, len(token1))
	}
}

func TestSecretName(t *testing.T) {
	if got := secretName("abc123"); got != "ghpat-abc123" {
		t.Errorf("expected %q, got %q", "ghpat-abc123", got)
	}
}

func readSecretData(t *testing.T, store *Store, token string) secretData {
	t.Helper()

	secret, err := store.client.CoreV1().Secrets(testNamespace).Get(context.Background(), secretName(token), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("session secret not found: %v", err)
	}
	var data secretData
	if err := json.Unmarshal(secret.Data["session"], &data); err != nil {
		t.Fatalf("unmarshal session data: %v", err)
	}
	return data
}

func expiredSecret(token string) *corev1.Secret {
	data, err := json.Marshal(secretData{
		Token:     "ghp_secret",
		Type:      CredentialTypePAT,
		Owner:     "alice-gh",
		User:      alice,
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		panic(err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName(token), Namespace: testNamespace},
		Data:       map[string][]byte{"session": data},
	}
}
