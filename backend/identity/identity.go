// Package identity answers "which OpenShift user is behind this request?".
// The console proxy forwards the console user's bearer token, but the token is
// opaque, so the only way to learn the identity is to ask the API server.
package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/openshift/faas-console-plugin/backend/kube"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// User is the OpenShift user the API server sees behind a bearer token.
type User struct {
	Username string `json:"username"`
	UID      string `json:"uid,omitempty"`
}

// Matches reports whether u and other are the same OpenShift user.
//
// The UID is the authoritative key where it exists: a username can be deleted
// and recreated for a different person, a UID is never reused. kube:admin is
// backed by a static Secret rather than a User object and therefore has no UID,
// so for it the comparison falls back to the name. Two empty users never match,
// which keeps an unbound session from being readable by everybody.
func (u User) Matches(other User) bool {
	if u.UID != "" || other.UID != "" {
		return u.UID == other.UID && u.Username == other.Username
	}
	return u.Username != "" && u.Username == other.Username
}

// Resolver turns a bearer token into the user it authenticates as.
type Resolver interface {
	Resolve(ctx context.Context, token string) (User, error)
}

// cacheTTL bounds how long a token-to-user answer is reused. Without it every
// backend call would cost one extra API server round trip just to learn the
// caller's name. Console tokens outlive this by hours, so the window only
// delays noticing a revoked token, which the API server rejects anyway on the
// next call the handler makes with it.
const cacheTTL = 5 * time.Minute

// NewResolver builds a resolver backed by SelfSubjectReview, the same call
// `oc auth whoami` makes. Every authenticated user may create one. host is
// empty in the pod, where the in-cluster config supplies the address.
func NewResolver(host string, caCert []byte) Resolver {
	return &reviewResolver{host: host, caCert: caCert, entries: map[string]cacheEntry{}}
}

type reviewResolver struct {
	host   string
	caCert []byte

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	user      User
	expiresAt time.Time
}

func (r *reviewResolver) Resolve(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, fmt.Errorf("no bearer token")
	}

	key := cacheKey(token)
	if user, ok := r.lookup(key); ok {
		return user, nil
	}

	cfg, err := kube.RESTConfig(r.host, token, r.caCert)
	if err != nil {
		return User{}, fmt.Errorf("build rest config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return User{}, fmt.Errorf("create kubernetes client: %w", err)
	}

	review, err := client.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return User{}, fmt.Errorf("self subject review: %w", err)
	}

	user := User{Username: review.Status.UserInfo.Username, UID: review.Status.UserInfo.UID}
	if user.Username == "" {
		return User{}, fmt.Errorf("self subject review returned no username")
	}

	r.store(key, user)
	return user, nil
}

// cacheKey hashes the token so it is never held in a map key that could end up
// in a heap dump or a debug print.
func cacheKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (r *reviewResolver) lookup(key string) (User, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return User{}, false
	}
	return entry.user, true
}

func (r *reviewResolver) store(key string, user User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Nothing ever removes an entry explicitly, so drop the stale ones here or
	// the map grows with every console session the backend has ever seen.
	now := time.Now()
	for k, entry := range r.entries {
		if now.After(entry.expiresAt) {
			delete(r.entries, k)
		}
	}

	r.entries[key] = cacheEntry{user: user, expiresAt: now.Add(cacheTTL)}
}

// ResolverStub is a test double. It resolves every token to the same user
// unless OnResolve says otherwise.
type ResolverStub struct {
	OnResolve func(ctx context.Context, token string) (User, error)
}

func (s *ResolverStub) Resolve(ctx context.Context, token string) (User, error) {
	if s.OnResolve != nil {
		return s.OnResolve(ctx, token)
	}
	return User{Username: "tester", UID: "tester-uid"}, nil
}
