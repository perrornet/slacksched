package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSessionWorkspace(t *testing.T) {
	root := t.TempDir()
	wsroot := filepath.Join(root, "w")
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", "AGENTS.md", "", "", "", SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := BuildSchedulerAgentConstraintsMarkdown("AGENTS.md", "") + "\n" + strings.TrimSpace(BuiltinAgentMarkdownFileIntro) + "\n" + SlackContextSectionHTMLComment("")
	if string(b) != want {
		t.Fatalf("got %q", b)
	}
}

func TestCreateSessionWorkspace_AgentMDAppend(t *testing.T) {
	root := t.TempDir()
	appendFile := filepath.Join(root, "append.md")
	if err := os.WriteFile(appendFile, []byte("\n## 用户附加说明\n\n只做最小修改。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wsroot := filepath.Join(root, "w")
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", "AGENTS.md", appendFile, "", "", SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "## 用户附加说明\n\n只做最小修改。\n") {
		t.Fatalf("expected appended markdown in AGENTS.md, got %q", got)
	}
	if !strings.Contains(got, "schduler-slack-context:start") {
		t.Fatalf("expected generated slack context block, got %q", got)
	}
}

func TestCreateSessionWorkspace_SlackMrkdwnGuide(t *testing.T) {
	root := t.TempDir()
	guide := filepath.Join(root, "guide.md")
	if err := os.WriteFile(guide, []byte("mrkdwn body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wsroot := filepath.Join(root, "w")
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", "AGENTS.md", "", guide, "", SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "references", "slack-mrkdwn-guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "mrkdwn body\n" {
		t.Fatalf("guide: got %q", b)
	}
}

func TestSessionBotIdentity_agentMarkdownSection(t *testing.T) {
	s := (SessionBotIdentity{
		UserID:      "U0BOT",
		BotID:       "B0BOT",
		UserName:    "mybot",
		DisplayName: "My Bot",
	}).agentMarkdownSection()
	if s == "" {
		t.Fatal("empty")
	}
	for _, x := range []string{"U0BOT", "B0BOT", "mybot", "My Bot", "<@U0BOT>"} {
		if x == "B0BOT" || x == "mybot" {
			continue
		}
		if !strings.Contains(s, x) {
			t.Fatalf("missing %q in %q", x, s)
		}
	}
}

func TestCreateSessionWorkspace_SessionBot(t *testing.T) {
	root := t.TempDir()
	wsroot := filepath.Join(root, "w")
	bot := SessionBotIdentity{
		UserID: "U0BOT", BotID: "B0BOT", UserName: "bot", DisplayName: "Bot",
	}
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", "AGENTS.md", "", "", "", bot)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "## 本会话的 Slack 机器人身份") || !strings.Contains(got, "U0BOT") {
		t.Fatalf("expected bot section in AGENTS.md, got %q", got)
	}
}

func TestCreateSessionWorkspace_ContextAPISection(t *testing.T) {
	root := t.TempDir()
	wsroot := filepath.Join(root, "w")
	apiBase := "http://127.0.0.1:19847"
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", "AGENTS.md", "", "", apiBase, SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "# 当前会话约束") || !strings.Contains(got, "schduler-slack-context:start") {
		t.Fatalf("expected generated constraints and slack context block, got %q", got)
	}
	if !strings.Contains(got, "## Slack 线程上下文 HTTP API") || !strings.Contains(got, apiBase) || !strings.Contains(got, "SCHDULER_CONTEXT_API_TOKEN") {
		t.Fatalf("expected generated context API section, got %q", got)
	}
}
