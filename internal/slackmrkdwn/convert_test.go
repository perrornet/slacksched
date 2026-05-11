package slackmrkdwn

import (
	"strings"
	"testing"
)

func TestCommonMarkdownToMrkdwn_GuideTable(t *testing.T) {
	if got := CommonMarkdownToMrkdwn("**bold**"); got != "*bold*" {
		t.Fatalf("bold: %q", got)
	}
	if got := CommonMarkdownToMrkdwn("__bold__"); got != "*bold*" {
		t.Fatalf("bold under: %q", got)
	}
	if got := CommonMarkdownToMrkdwn("~~del~~"); got != "~del~" {
		t.Fatalf("strike: %q", got)
	}
	if got := CommonMarkdownToMrkdwn("[Visit](https://example.com)"); got != "<https://example.com|Visit>" {
		t.Fatalf("link: %q", got)
	}
	if got := CommonMarkdownToMrkdwn("# Title"); got != "*Title*" {
		t.Fatalf("heading: %q", got)
	}
}

// goclaw pipeline does not map Markdown single-* emphasis to Slack _italic_;
// *word* passes through for Slack’s *bold* interpretation.
func TestSingleAsteriskUnchanged(t *testing.T) {
	if got := CommonMarkdownToMrkdwn("*emph*"); got != "*emph*" {
		t.Fatalf("got %q", got)
	}
}

func TestBoldThenSingleAsteriskRuns(t *testing.T) {
	got := CommonMarkdownToMrkdwn("**B** and *I*")
	if got != "*B* and *I*" {
		t.Fatalf("got %q", got)
	}
}

func TestFenceAndInlineCode(t *testing.T) {
	in := "Say `x` and:\n```python\na = 1\n```\nok"
	got := CommonMarkdownToMrkdwn(in)
	if !strings.Contains(got, "`x`") || !strings.Contains(got, "a = 1") || !strings.Contains(got, "```python") {
		t.Fatalf("got %q", got)
	}
}

// Markdown image ![alt](url): link regex matches [alt](url); leading ! remains (goclaw behavior).
func TestImageMarkdown(t *testing.T) {
	got := CommonMarkdownToMrkdwn("![x](https://x.org/y.png)")
	if want := "!<https://x.org/y.png|x>"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHTMLBoldToMrkdwn(t *testing.T) {
	got := CommonMarkdownToMrkdwn("<b>hi</b>")
	if got != "*hi*" {
		t.Fatalf("got %q", got)
	}
}

func TestSlackMentionPreserved(t *testing.T) {
	got := CommonMarkdownToMrkdwn("hey <@U12345678>")
	if got != "hey <@U12345678>" {
		t.Fatalf("got %q", got)
	}
}
