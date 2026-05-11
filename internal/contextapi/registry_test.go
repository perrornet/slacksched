package contextapi

import (
	"testing"

	"github.com/perrornet/slacksched/internal/session"
)

func TestRegistryRoundTrip(t *testing.T) {
	r := NewRegistry()
	k := session.Key{TeamID: "T1", ChannelID: "C1", RootThreadTS: "10.0"}
	r.Register("tok1", k)
	got, ok := r.Lookup("tok1")
	if !ok || got != k {
		t.Fatalf("lookup: ok=%v got=%+v", ok, got)
	}
	r.Unregister("tok1")
	if _, ok := r.Lookup("tok1"); ok {
		t.Fatal("expected unregister")
	}
}

func TestBearerToken(t *testing.T) {
	if bearerToken("Bearer abc") != "abc" {
		t.Fatal()
	}
	if bearerToken("bearer x") != "x" {
		t.Fatal()
	}
	if bearerToken("") != "" {
		t.Fatal()
	}
}
