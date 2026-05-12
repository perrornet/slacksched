package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceSlackContextBody(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "AGENTS.md")
	initial := "# top\n" + SlackContextSectionHTMLComment("old inner")
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceSlackContextBody(p, "line1\nline2"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "old inner") {
		t.Fatalf("old inner should be replaced: %q", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Fatalf("missing new inner: %q", got)
	}
	if !strings.Contains(got, slackContextStartMarker) || !strings.Contains(got, slackContextEndMarker) {
		t.Fatalf("markers lost: %q", got)
	}
}

func TestSlackRuntimeContext_BuildMarkdownBody(t *testing.T) {
	c := SlackRuntimeContext{
		AgentDoc:              "AGENTS.md",
		TeamID:                "T1",
		ChannelID:             "C9",
		ChannelName:           "general",
		IsIM:                  false,
		RootThreadTS:          "1.0",
		TriggerMessageTS:      "2.0",
		ContextAPIBaseURL:     "http://127.0.0.1:9",
		ThreadPriorTranscript: "prior line",
	}
	got := c.BuildMarkdownBody()
	if !strings.Contains(got, "T1") || !strings.Contains(got, "C9") || !strings.Contains(got, "#general") {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(got, "prior line") {
		t.Fatalf("missing transcript: %q", got)
	}
	c2 := SlackRuntimeContext{ChannelID: "D1", IsIM: true, TeamID: "T", RootThreadTS: "a", TriggerMessageTS: "b"}
	got2 := c2.BuildMarkdownBody()
	if !strings.Contains(got2, "私信") {
		t.Fatalf("%q", got2)
	}
}
