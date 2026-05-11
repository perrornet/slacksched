package slackapp

import (
	"testing"
)

func TestBuildAssistantLoadingCarousel_Len(t *testing.T) {
	ms := buildAssistantLoadingCarousel("bootstrap", "")
	if len(ms) < 2 || len(ms) > 10 {
		t.Fatalf("len=%d", len(ms))
	}
	ms2 := buildAssistantLoadingCarousel("tool_call", "read")
	if len(ms2) == 0 {
		t.Fatal()
	}
	if ms2[0] != "Reading a file…" {
		t.Fatalf("first line %q", ms2[0])
	}
}

func TestCarouselToolPhrase_UnknownToolUsesID(t *testing.T) {
	if s := streamPhaseCarouselDetail("tool_call", "user-github_search_issues"); s != "Calling user-github_search_issues…" {
		t.Fatalf("%q", s)
	}
}

func TestBuildLiveStatusLine_Tools(t *testing.T) {
	if s := buildLiveStatusLine("Working on your request…", []string{"glob", "read"}); s != "Working on your request… — tools: glob, read" {
		t.Fatalf("%q", s)
	}
	if s := buildLiveStatusLine("Working…", nil); s != "Working…" {
		t.Fatalf("%q", s)
	}
}
