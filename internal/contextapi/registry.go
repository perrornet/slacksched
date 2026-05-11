package contextapi

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/perrornet/slacksched/internal/session"
)

// Registry maps a session-only bearer token to a Slack thread key.
type Registry struct {
	mu sync.RWMutex
	m  map[string]session.Key
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[string]session.Key)}
}

// Register binds token to key until Unregister. Token must be non-empty.
func (r *Registry) Register(token string, key session.Key) {
	if r == nil || token == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[token] = key
}

// Unregister removes a token.
func (r *Registry) Unregister(token string) {
	if r == nil || token == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, token)
}

// Lookup returns the session key for a valid token.
func (r *Registry) Lookup(token string) (session.Key, bool) {
	if r == nil || token == "" {
		return session.Key{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.m[token]
	return k, ok
}

// NewToken returns a random hex string suitable as a bearer token.
func NewToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString(b[:])
	}
	return hex.EncodeToString(b[:])
}
