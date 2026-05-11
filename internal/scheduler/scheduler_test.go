package scheduler

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/perrornet/slacksched/internal/config"
	"github.com/perrornet/slacksched/internal/session"
	"github.com/perrornet/slacksched/internal/workspace"
)

type fakeRunner struct {
	id   string
	n    atomic.Int32
	fail error
}

func (f *fakeRunner) SessionID() string { return f.id }

func (f *fakeRunner) Prompt(ctx context.Context, userText string) (string, string, error) {
	f.n.Add(1)
	if f.fail != nil {
		return "", "", f.fail
	}
	return userText + "!", "end_turn", nil
}

func (f *fakeRunner) Close() error { return nil }

type fakeFactory struct {
	started atomic.Int32
}

func (ff *fakeFactory) Start(ctx context.Context, log *slog.Logger, prof config.ProviderProfile, absWorkspace string, extraEnv []string) (PromptRunner, error) {
	ff.started.Add(1)
	return &fakeRunner{id: "fake"}, nil
}

func TestSchedulerFIFOAndSingleFactory(t *testing.T) {
	dir := t.TempDir()
	tpl := filepath.Join(dir, "t.md")
	if err := os.WriteFile(tpl, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
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
  agent_md_template_path: ` + tpl + `
  agent_md_filename: AGENTS.md
  append_system_prompt: ""
  provider_idle_timeout: 20m
  provider_shutdown_timeout: 5s
  session_idle_timeout: 20m
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
	c, err := config.Load(yml)
	if err != nil {
		t.Fatal(err)
	}
	ff := &fakeFactory{}
	sch, err := New(c, slog.Default(), ff, nil, "", workspace.SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{TeamID: "T", ChannelID: "C", RootThreadTS: "1.0"}
	ch := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		i := i
		sch.Enqueue(Job{
			Key: key,
			Text: string(rune('a' + i)),
			Done: func(text string, err error) {
				if err != nil {
					t.Error(err)
				}
				_ = text
				ch <- struct{}{}
			},
		})
	}
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}
	if ff.started.Load() != 1 {
		t.Fatalf("expected 1 provider start, got %d", ff.started.Load())
	}
}
