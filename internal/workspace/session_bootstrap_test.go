package workspace

import (
	"strings"
	"testing"
)

func TestBuildSessionBootstrapMarkdown_nonEmpty(t *testing.T) {
	s := BuildSessionBootstrapMarkdown()
	if s == "" || !strings.Contains(s, "schduler") || !strings.Contains(s, "AGENTS.md") {
		t.Fatalf("unexpected bootstrap: %q", s)
	}
}
