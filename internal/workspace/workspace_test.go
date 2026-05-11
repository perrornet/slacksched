package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSessionWorkspace(t *testing.T) {
	root := t.TempDir()
	tpl := filepath.Join(root, "tpl.md")
	if err := os.WriteFile(tpl, []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wsroot := filepath.Join(root, "w")
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", tpl, "AGENTS.md", "append\n", "", SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "# T\n\nappend\n" {
		t.Fatalf("got %q", b)
	}
}

func TestCreateSessionWorkspace_SlackMrkdwnGuide(t *testing.T) {
	root := t.TempDir()
	tpl := filepath.Join(root, "tpl.md")
	if err := os.WriteFile(tpl, []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	guide := filepath.Join(root, "guide.md")
	if err := os.WriteFile(guide, []byte("mrkdwn body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wsroot := filepath.Join(root, "w")
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", tpl, "AGENTS.md", "", guide, SessionBotIdentity{})
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
		if !strings.Contains(s, x) {
			t.Fatalf("missing %q in %q", x, s)
		}
	}
}

func TestCreateSessionWorkspace_SessionBot(t *testing.T) {
	root := t.TempDir()
	tpl := filepath.Join(root, "tpl.md")
	if err := os.WriteFile(tpl, []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wsroot := filepath.Join(root, "w")
	bot := SessionBotIdentity{
		UserID: "U0BOT", BotID: "B0BOT", UserName: "bot", DisplayName: "Bot",
	}
	p, err := CreateSessionWorkspace(wsroot, "T1", "C1", "1234.5678", "abcd", tpl, "AGENTS.md", "", "", bot)
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
