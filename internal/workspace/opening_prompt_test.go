package workspace

import (
	"strings"
	"testing"
)

func TestBuildSessionOpeningPrompt_replacesPlaceholders(t *testing.T) {
	raw := "c=<slack-channel-id> n=<slack-channel-name> u=<mention-user-id>\n<mention-user-message>"
	sc := SlackRuntimeContext{
		ChannelID:   "C1",
		ChannelName: "general",
		IsIM:        false,
	}
	s := buildSessionOpeningFromTemplate(raw, sc, "U99", "hello")
	want := "c=C1 n=general u=U99\nhello"
	if s != want {
		t.Fatalf("got %q want %q", s, want)
	}
}

func TestBuildSessionOpeningPrompt_appendsMessageWhenNoPlaceholder(t *testing.T) {
	sc := SlackRuntimeContext{ChannelID: "C", ChannelName: "", IsIM: true}
	s := buildSessionOpeningFromTemplate("prefix", sc, "U1", "tail")
	want := "prefix\n\ntail"
	if s != want {
		t.Fatalf("got %q want %q", s, want)
	}
}

func TestBuildSessionOpeningPrompt_emptyTemplateUserOnly(t *testing.T) {
	sc := SlackRuntimeContext{}
	s := buildSessionOpeningFromTemplate("", sc, "", "only")
	if s != "only" {
		t.Fatalf("got %q", s)
	}
}

func TestBuildSessionOpeningPrompt_builtinContainsMentionSlot(t *testing.T) {
	if !strings.Contains(BuiltinAgentMarkdownTemplate, "<mention-user-message>") {
		t.Fatal("builtin template missing user message placeholder")
	}
}

func TestBuildSessionOpeningPrompt_includesThreadHistoryWhenPresent(t *testing.T) {
	sc := SlackRuntimeContext{
		ChannelID:             "C1",
		ChannelName:           "general",
		ThreadPriorTranscript: "[时间戳 1.0] U1: 之前的消息",
	}
	s := BuildSessionOpeningPrompt(sc, "U99", "当前消息")
	if !strings.Contains(s, "线程内先前消息") {
		t.Fatalf("missing history heading: %q", s)
	}
	if !strings.Contains(s, sc.ThreadPriorTranscript) {
		t.Fatalf("missing thread history: %q", s)
	}
	if !strings.Contains(s, "当前消息") {
		t.Fatalf("missing current message: %q", s)
	}
}

func TestBuildSessionOpeningPrompt_quotesHistorySafely(t *testing.T) {
	sc := SlackRuntimeContext{
		ChannelID:             "C1",
		ChannelName:           "general",
		ThreadPriorTranscript: "第一行\n```go\nfmt.Println(\"hi\")\n```",
	}
	s := BuildSessionOpeningPrompt(sc, "U99", "当前消息")
	if strings.Contains(s, "```text") {
		t.Fatalf("history should not use fenced block: %q", s)
	}
	if !strings.Contains(s, "> ```go") {
		t.Fatalf("history code fence should be quoted: %q", s)
	}
}
