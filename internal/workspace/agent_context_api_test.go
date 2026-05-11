package workspace

import (
	"strings"
	"testing"
)

func TestBuildAgentContextAPISectionMarkdown_empty(t *testing.T) {
	if BuildAgentContextAPISectionMarkdown("") != "" {
		t.Fatal("expected empty")
	}
	if BuildAgentContextAPISectionMarkdown(" \t ") != "" {
		t.Fatal("expected empty for whitespace")
	}
}

func TestBuildAgentContextAPISectionMarkdown_trimsSlash(t *testing.T) {
	s := BuildAgentContextAPISectionMarkdown("http://127.0.0.1:1/")
	if !strings.Contains(s, "http://127.0.0.1:1/v1/slack/thread/messages") {
		t.Fatalf("missing path: %s", s)
	}
	if strings.Contains(s, "http://127.0.0.1:1//v1") {
		t.Fatal("double slash")
	}
}

func TestBuildAgentContextAPISectionMarkdown_allowlistFromBinary(t *testing.T) {
	s := BuildAgentContextAPISectionMarkdown("http://127.0.0.1:9")
	for _, needle := range []string{"自动生成", "conversations.replies", "users.info", "files.info", "/v1/slack/web-api/"} {
		if !strings.Contains(s, needle) {
			t.Fatalf("missing %q in doc", needle)
		}
	}
}
