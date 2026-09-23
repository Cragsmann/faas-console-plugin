package identity

import "testing"

func TestUserMatches(t *testing.T) {
	tests := []struct {
		name  string
		a, b  User
		match bool
	}{
		{
			name:  "same username and uid",
			a:     User{Username: "alice", UID: "uid-1"},
			b:     User{Username: "alice", UID: "uid-1"},
			match: true,
		},
		{
			// kube:admin is backed by a static Secret, not a User object, so
			// the API server reports no UID for it.
			name:  "same username, neither has a uid",
			a:     User{Username: "kube:admin"},
			b:     User{Username: "kube:admin"},
			match: true,
		},
		{
			// A deleted and recreated account keeps the name but gets a fresh
			// UID, and must not inherit the old user's sessions.
			name:  "same username, different uid",
			a:     User{Username: "alice", UID: "uid-1"},
			b:     User{Username: "alice", UID: "uid-2"},
			match: false,
		},
		{
			name:  "different username, same uid",
			a:     User{Username: "alice", UID: "uid-1"},
			b:     User{Username: "mallory", UID: "uid-1"},
			match: false,
		},
		{
			name:  "uid known on one side only",
			a:     User{Username: "alice", UID: "uid-1"},
			b:     User{Username: "alice"},
			match: false,
		},
		{
			// Guards sessions written before the binding existed: an empty
			// stored user must not match an empty caller.
			name:  "both empty",
			a:     User{},
			b:     User{},
			match: false,
		},
		{
			name:  "empty against a real user",
			a:     User{},
			b:     User{Username: "alice", UID: "uid-1"},
			match: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Matches(tc.b); got != tc.match {
				t.Errorf("%+v.Matches(%+v) = %v, want %v", tc.a, tc.b, got, tc.match)
			}
			if got := tc.b.Matches(tc.a); got != tc.match {
				t.Errorf("Matches should be symmetric: %+v.Matches(%+v) = %v, want %v", tc.b, tc.a, got, tc.match)
			}
		})
	}
}

func TestResolveRejectsEmptyToken(t *testing.T) {
	resolver := NewResolver("https://api.example.com:6443", nil)

	if _, err := resolver.Resolve(t.Context(), ""); err == nil {
		t.Error("Resolve should reject an empty token instead of calling the API server")
	}
}

func TestCacheKeyHidesTheToken(t *testing.T) {
	const token = "sha256~secret-console-token"

	key := cacheKey(token)

	if key == token {
		t.Error("cache key should be a hash, not the token itself")
	}
	if key != cacheKey(token) {
		t.Error("cache key should be stable for the same token")
	}
	if key == cacheKey(token+"x") {
		t.Error("different tokens should produce different cache keys")
	}
}
