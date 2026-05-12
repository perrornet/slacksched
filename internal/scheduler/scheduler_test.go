package scheduler

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

type capturingRunner struct {
	prompts []string
}

func (c *capturingRunner) SessionID() string { return "cap" }

func (c *capturingRunner) Prompt(ctx context.Context, userText string) (string, string, error) {
	c.prompts = append(c.prompts, userText)
	return userText + "!", "end_turn", nil
}

func (c *capturingRunner) Close() error { return nil }

type capturingFactory struct {
	r         *capturingRunner
	workspace string
	started   atomic.Int32
}

func (cf *capturingFactory) Start(ctx context.Context, log *slog.Logger, prof config.ProviderProfile, absWorkspace string, extraEnv []string) (PromptRunner, error) {
	cf.started.Add(1)
	cf.workspace = absWorkspace
	return cf.r, nil
}

func TestSessionBootstrapSendsTwoPromptsThenSingle(t *testing.T) {
	dir := t.TempDir()
	tpl := filepath.Join(dir, "t.md")
	if err := os.WriteFile(tpl, []byte("tpl\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantBoot := workspace.BuildSessionBootstrapMarkdown()
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
	cap := &capturingRunner{}
	cf := &capturingFactory{r: cap}
	sch, err := New(c, slog.Default(), cf, nil, "", workspace.SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{TeamID: "T", ChannelID: "C", RootThreadTS: "1.0"}
	ch := make(chan struct{}, 2)
	sch.Enqueue(Job{
		Key:                  key,
		Text:                 "WRAPPED_FIRST",
		AfterBootstrapPrompt: "PLAIN_USER",
		Done: func(text string, err error) {
			if err != nil {
				t.Errorf("first job: %v", err)
			}
			if want := "PLAIN_USER!"; text != want {
				t.Errorf("slack reply = %q want %q", text, want)
			}
			ch <- struct{}{}
		},
	})
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout first job")
	}
	if len(cap.prompts) != 2 {
		t.Fatalf("prompts = %v (%d calls)", cap.prompts, len(cap.prompts))
	}
	if cap.prompts[0] != wantBoot {
		t.Fatalf("first prompt mismatch\n got (len=%d) want (len=%d)", len(cap.prompts[0]), len(wantBoot))
	}
	if cap.prompts[1] != "PLAIN_USER" {
		t.Fatalf("second prompt = %q", cap.prompts[1])
	}

	sch.Enqueue(Job{
		Key:  key,
		Text: "SECOND_MSG",
		Done: func(text string, err error) { ch <- struct{}{} },
	})
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout second job")
	}
	if len(cap.prompts) != 3 || cap.prompts[2] != "SECOND_MSG" {
		t.Fatalf("after second job prompts = %v", cap.prompts)
	}
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
			Key:  key,
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

func TestPreSessionCommandRunsBeforeProviderStart(t *testing.T) {
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
  provider_idle_timeout: 20m
  provider_shutdown_timeout: 5s
  session_idle_timeout: 20m
  prompt_timeout: 1m
  workspace_retention: delete_on_session_close
  pre_session_command: 'printf "%s\n%s\n%s\n%s\n%s\n" "$PWD" "$SCHDULER_SESSION_WORKSPACE" "$SCHDULER_SESSION_TEAM_ID" "$SCHDULER_SESSION_CHANNEL_ID" "$SCHDULER_SESSION_ROOT_THREAD_TS" > hook.env'
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
	cap := &capturingRunner{}
	cf := &capturingFactory{r: cap}
	sch, err := New(c, slog.Default(), cf, nil, "", workspace.SessionBotIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{TeamID: "T", ChannelID: "C", RootThreadTS: "1.0"}
	ch := make(chan error, 1)
	sch.Enqueue(Job{
		Key:  key,
		Text: "hello",
		Done: func(text string, err error) {
			ch <- err
		},
	})
	select {
	case err := <-ch:
		if err != nil {
			t.Fatalf("job failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	if cf.started.Load() != 1 {
		t.Fatalf("expected provider start, got %d", cf.started.Load())
	}
	data, err := os.ReadFile(filepath.Join(cf.workspace, "hook.env"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{cf.workspace, cf.workspace, "T", "C", "1.0"}
	if len(lines) != len(want) {
		t.Fatalf("hook.env lines = %q", lines)
	}
	gotPWD, err := filepath.EvalSymlinks(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	wantPWD, err := filepath.EvalSymlinks(cf.workspace)
	if err != nil {
		t.Fatal(err)
	}
	if gotPWD != wantPWD {
		t.Fatalf("hook cwd = %q want %q", gotPWD, wantPWD)
	}
	for i := 1; i < len(want); i++ {
		if lines[i] != want[i] {
			t.Fatalf("hook.env line %d = %q want %q", i, lines[i], want[i])
		}
	}
}

func TestPreSessionCommandFailureStopsProviderStart(t *testing.T) {
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
  provider_idle_timeout: 20m
  provider_shutdown_timeout: 5s
  session_idle_timeout: 20m
  prompt_timeout: 1m
  workspace_retention: delete_on_session_close
  pre_session_command: 'echo hook failed >&2; exit 7'
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
	ch := make(chan error, 1)
	sch.Enqueue(Job{
		Key:  session.Key{TeamID: "T", ChannelID: "C", RootThreadTS: "1.0"},
		Text: "hello",
		Done: func(text string, err error) {
			ch <- err
		},
	})
	select {
	case err := <-ch:
		if err == nil {
			t.Fatal("expected pre-session command error")
		}
		if !strings.Contains(err.Error(), "pre-session command") {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	if ff.started.Load() != 0 {
		t.Fatalf("expected no provider start, got %d", ff.started.Load())
	}
}
