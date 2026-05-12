package slackapp

import (
	"strings"
	"testing"
)

func TestBuildSlackTurnEnvelope_quotingAndIDs(t *testing.T) {
	got := buildSlackTurnEnvelope("AGENTS.md", "T1", "C1", "10.0", "11.0", "Ev1", "U1", "hello\nline2")
	if !strings.Contains(got, "[新消息]") || !strings.Contains(got, "`team_id`: `T1`") {
		t.Fatalf("missing framing: %q", got)
	}
	if !strings.Contains(got, "> hello\n> line2") {
		t.Fatalf("expected blockquoted body: %q", got)
	}
	if strings.Contains(got, "hello\n\n> line2") {
		t.Fatal("unexpected paragraph break in quote")
	}
}

func TestBuildSlackTurnEnvelope_emptyBody(t *testing.T) {
	got := buildSlackTurnEnvelope("x.md", "T", "C", "a", "b", "e", "", "")
	if !strings.Contains(got, "> ") {
		t.Fatalf("%q", got)
	}
	if strings.Contains(got, "- `user_id`:") {
		t.Fatal("should omit empty user_id")
	}
}
