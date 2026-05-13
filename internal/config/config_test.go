package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMinimal(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, "c.yaml")
	body := `
slack:
  bot_token_env: SLACK_BOT_TOKEN
  app_token_env: SLACK_APP_TOKEN
  allowed_dm_user_ids: []
  allowed_channel_ids: []
  require_mention_in_channels: true
scheduler:
  workspaces_root: ` + filepath.Join(dir, "ws") + `
  agent_md_filename: AGENTS.md
  provider_idle_timeout: 1m
  provider_shutdown_timeout: 5s
  session_idle_timeout: 1m
  prompt_timeout: 1m
  workspace_retention: delete_on_session_close
providers:
  default: p1
  profiles:
    p1:
      command: echo
      args: []
`
	if err := os.WriteFile(yml, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("SLACK_BOT_TOKEN", "xoxb-test"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("SLACK_APP_TOKEN", "xapp-test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("SLACK_BOT_TOKEN")
		_ = os.Unsetenv("SLACK_APP_TOKEN")
	})
	c, err := Load(yml)
	if err != nil {
		t.Fatal(err)
	}
	name, err := c.DefaultProviderProfile()
	if err != nil {
		t.Fatal(err)
	}
	if name != "p1" {
		t.Fatal(c.Providers.Default)
	}
	if !c.Slack.ConvertOutboundMarkdownEnabled() {
		t.Fatal("default convert_outbound should be enabled when unset")
	}
	if !c.Slack.TurnEnvelopeEnabled() {
		t.Fatal("default turn_envelope should be on when unset")
	}
}

func TestLoadPreSessionCommand(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, "c.yaml")
	body := `
slack:
  bot_token_env: SLACK_BOT_TOKEN
  app_token_env: SLACK_APP_TOKEN
  allowed_dm_user_ids: []
  allowed_channel_ids: []
  require_mention_in_channels: true
scheduler:
  workspaces_root: ` + filepath.Join(dir, "ws") + `
  agent_md_filename: AGENTS.md
  provider_idle_timeout: 1m
  provider_shutdown_timeout: 5s
  session_idle_timeout: 1m
  prompt_timeout: 1m
  workspace_retention: delete_on_session_close
  pre_session_command: 'printf hook > .session-hook'
providers:
  default: p1
  profiles:
    p1:
      command: echo
      args: []
`
	if err := os.WriteFile(yml, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(yml)
	if err != nil {
		t.Fatal(err)
	}
	if c.Scheduler.PreSessionCommand != "printf hook > .session-hook" {
		t.Fatalf("pre_session_command = %q", c.Scheduler.PreSessionCommand)
	}
}

func TestTurnEnvelopeExplicitFalse(t *testing.T) {
	f := &SlackConfig{}
	if !f.TurnEnvelopeEnabled() {
		t.Fatal("nil TurnEnvelope should default on")
	}
	off := false
	f.TurnEnvelope = &off
	if f.TurnEnvelopeEnabled() {
		t.Fatal("explicit false should disable")
	}
}
