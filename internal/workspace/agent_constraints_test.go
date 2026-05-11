package workspace

import (
	"strings"
	"testing"
)

func TestBuildSchedulerAgentConstraints_withContextAPI(t *testing.T) {
	s := BuildSchedulerAgentConstraintsMarkdown("AGENTS.md", "http://127.0.0.1:1")
	if !strings.Contains(s, "SCHDULER_CONTEXT_API_URL") || !strings.Contains(s, "AGENTS.md") || !strings.Contains(s, "文末") {
		t.Fatalf("%q", s)
	}
}

func TestBuildSchedulerAgentConstraints_noContextAPI(t *testing.T) {
	s := BuildSchedulerAgentConstraintsMarkdown("rules.md", "")
	if strings.Contains(s, "SCHDULER_CONTEXT_API_URL") {
		t.Fatal("unexpected env mention")
	}
	if !strings.Contains(s, "未启用") {
		t.Fatalf("%q", s)
	}
}
