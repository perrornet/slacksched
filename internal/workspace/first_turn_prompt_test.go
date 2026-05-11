package workspace

import "testing"

func TestComposeFirstTurnPrompt(t *testing.T) {
	got := ComposeFirstTurnPrompt("A", "B")
	if got != "A\n\n---\n\nB" {
		t.Fatalf("%q", got)
	}
	if ComposeFirstTurnPrompt("", "x") != "x" || ComposeFirstTurnPrompt("  ", "x") != "x" {
		t.Fatal("empty prefix")
	}
	if ComposeFirstTurnPrompt("H", "") != "H" {
		t.Fatal("empty slack")
	}
}
