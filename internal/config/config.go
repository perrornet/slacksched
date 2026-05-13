package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level scheduler configuration.
type Config struct {
	Logging   LoggingConfig   `yaml:"logging"`
	Slack     SlackConfig     `yaml:"slack"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Providers ProvidersConfig `yaml:"providers"`
}

// LoggingConfig controls process log level and verbose tracing.
type LoggingConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error; empty defaults to info
	ACPTrace   bool   `yaml:"acp_trace"`   // log every newline-delimited JSON-RPC line on provider stdio
	SlackTrace bool   `yaml:"slack_trace"` // include truncated inbound/outbound Slack message text in logs
	// FilePath when set, duplicate the same text logs to this file (append, create if missing).
	FilePath string `yaml:"file_path"`
}

// SlackConfig holds Slack Socket Mode and routing options.
type SlackConfig struct {
	BotTokenEnv           string   `yaml:"bot_token_env"`
	AppTokenEnv           string   `yaml:"app_token_env"`
	DefaultReplyBroadcast bool     `yaml:"default_reply_broadcast"`
	AllowedDMUserIDs      []string `yaml:"allowed_dm_user_ids"`
	AllowedChannelIDs     []string `yaml:"allowed_channel_ids"`
	// RequireMentionInChannels: in workspace channels, every message (including thread follow-ups) must @ the bot; ignored for IMs.
	RequireMentionInChannels bool `yaml:"require_mention_in_channels"`
	// AssistantStatus enables assistant.threads.setStatus while the provider runs (cleared when chat.postMessage succeeds).
	AssistantStatus          bool     `yaml:"assistant_status"`
	AssistantStatusText      string   `yaml:"assistant_status_text"`      // empty → code default "Working on your request…" (shown as "<app> is …"); live mode may append "— tools: …"
	AssistantLoadingMessages []string `yaml:"assistant_loading_messages"` // optional rotating loading lines (max 10)
	// AssistantLiveStatus: when true, map streaming events to assistant.threads.setStatus (Cursor stream-json; Codex app-server item/*).
	// Ignores assistant_loading_messages while the run is in progress (live labels take over).
	AssistantLiveStatus bool `yaml:"assistant_live_status"`
	// ThreadRepliesInPrompt when true, loads thread history (conversations.replies) into
	// the "Slack 会话上下文" section of AGENTS.md before each prompt (not into the user message).
	ThreadRepliesInPrompt bool `yaml:"thread_replies_in_prompt"`
	// ThreadRepliesMaxMessages caps messages collected from pagination (0 = default 100).
	ThreadRepliesMaxMessages int `yaml:"thread_replies_max_messages"`
	// ThreadRepliesMaxChars caps transcript size (0 = default 12000).
	ThreadRepliesMaxChars int `yaml:"thread_replies_max_chars"`
	// ConvertOutboundMarkdown when nil or true, maps agent-style Markdown to Slack mrkdwn on outbound replies (see references/slack-mrkdwn-guide.md).
	ConvertOutboundMarkdown *bool `yaml:"convert_outbound_markdown"`
	// ContextAPIListen starts a local HTTP API for on-demand thread history (e.g. 127.0.0.1:9847). Empty disables it.
	ContextAPIListen string `yaml:"context_api_listen"`
	// TurnEnvelope wraps each user message in structured framing (ids, “focus on this message”, blockquoted verbatim body).
	// When nil or true (default), envelope is on; set false to send raw Slack text only.
	TurnEnvelope *bool `yaml:"turn_envelope"`
}

// ConvertOutboundMarkdownEnabled is true unless convert_outbound_markdown is explicitly false.
func (s *SlackConfig) ConvertOutboundMarkdownEnabled() bool {
	if s == nil {
		return true
	}
	if s.ConvertOutboundMarkdown == nil {
		return true
	}
	return *s.ConvertOutboundMarkdown
}

// TurnEnvelopeEnabled is true unless turn_envelope is explicitly false.
func (s *SlackConfig) TurnEnvelopeEnabled() bool {
	if s == nil || s.TurnEnvelope == nil {
		return true
	}
	return *s.TurnEnvelope
}

// SchedulerConfig holds session lifecycle and workspace options.
type SchedulerConfig struct {
	WorkspacesRoot          string   `yaml:"workspaces_root"`
	AgentMDFilename         string   `yaml:"agent_md_filename"`
	AgentMDAppendPath       string   `yaml:"agent_md_append_path"`
	PreSessionCommand       string   `yaml:"pre_session_command"`
	ProviderIdleTimeout     Duration `yaml:"provider_idle_timeout"`
	ProviderShutdownTimeout Duration `yaml:"provider_shutdown_timeout"`
	SessionIdleTimeout      Duration `yaml:"session_idle_timeout"`
	PromptTimeout           Duration `yaml:"prompt_timeout"`
	WorkspaceRetention      string   `yaml:"workspace_retention"`
	// SlackMrkdwnGuidePath is optional. When set, that file is copied into each new session workspace
	// as references/slack-mrkdwn-guide.md (optional bundled mrkdwn reference for outbound conversion).
	SlackMrkdwnGuidePath string `yaml:"slack_mrkdwn_guide_path"`
}

// ProvidersConfig selects a default profile and defines commands.
type ProvidersConfig struct {
	Default  string                     `yaml:"default"`
	Profiles map[string]ProviderProfile `yaml:"profiles"`
}

// ProviderProfile describes how to launch a provider process.
// Transport "acp" (default): long-lived stdio JSON-RPC Agent Client Protocol.
// Transport "cursor_cli": cursor-agent chat -p … per Cursor CLI (see provider/cursor_args.go).
// Transport "codex_app_server": codex app-server --listen stdio:// (JSON-RPC over stdio).
type ProviderProfile struct {
	Transport string            `yaml:"transport"` // acp | cursor_cli | codex_app_server; empty means acp
	Command   string            `yaml:"command"`
	Args      []string          `yaml:"args"`  // acp: argv after command; cursor_cli/codex_app_server: extra args (filtered)
	Model     string            `yaml:"model"` // cursor_cli: --model; codex_app_server: thread/start model
	Mode      string            `yaml:"mode"`
	Env       map[string]string `yaml:"env"`
}

// Load reads and validates configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := c.applyEnvOverrides(); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyEnvOverrides() error {
	// Tokens resolved at runtime via env names in Slack app; no inline secrets in struct.
	return nil
}

// SlackBotToken returns the bot token from the configured env var.
func (c *Config) SlackBotToken() (string, error) {
	name := strings.TrimSpace(c.Slack.BotTokenEnv)
	if name == "" {
		name = "SLACK_BOT_TOKEN"
	}
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("environment variable %s is not set", name)
	}
	return v, nil
}

// SlackAppToken returns the app-level token for Socket Mode.
func (c *Config) SlackAppToken() (string, error) {
	name := strings.TrimSpace(c.Slack.AppTokenEnv)
	if name == "" {
		name = "SLACK_APP_TOKEN"
	}
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("environment variable %s is not set", name)
	}
	return v, nil
}

// SlogLevel maps logging.level to slog.Level (default info).
func (c *Config) SlogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(c.Logging.Level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// DefaultProviderProfile returns the configured default provider name.
func (c *Config) DefaultProviderProfile() (string, error) {
	name := strings.TrimSpace(c.Providers.Default)
	if name == "" {
		return "", fmt.Errorf("providers.default is required")
	}
	if _, ok := c.Providers.Profiles[name]; !ok {
		return "", fmt.Errorf("providers.default %q has no profiles.%s entry", name, name)
	}
	return name, nil
}

// Validate checks required fields and paths.
func (c *Config) Validate() error {
	root := strings.TrimSpace(c.Scheduler.WorkspacesRoot)
	if root == "" {
		return fmt.Errorf("scheduler.workspaces_root is required")
	}
	if strings.TrimSpace(c.Scheduler.AgentMDFilename) == "" {
		return fmt.Errorf("scheduler.agent_md_filename is required")
	}
	appendPath := strings.TrimSpace(c.Scheduler.AgentMDAppendPath)
	if appendPath != "" {
		if st, err := os.Stat(appendPath); err != nil || st.IsDir() {
			return fmt.Errorf("scheduler.agent_md_append_path must be a readable file: %s", appendPath)
		}
	}
	guide := strings.TrimSpace(c.Scheduler.SlackMrkdwnGuidePath)
	if guide != "" {
		if st, err := os.Stat(guide); err != nil || st.IsDir() {
			return fmt.Errorf("scheduler.slack_mrkdwn_guide_path must be a readable file: %s", guide)
		}
	}
	if c.Scheduler.ProviderIdleTimeout.Duration() <= 0 {
		return fmt.Errorf("scheduler.provider_idle_timeout must be positive")
	}
	if c.Scheduler.SessionIdleTimeout.Duration() <= 0 {
		return fmt.Errorf("scheduler.session_idle_timeout must be positive")
	}
	if c.Scheduler.ProviderShutdownTimeout.Duration() <= 0 {
		return fmt.Errorf("scheduler.provider_shutdown_timeout must be positive")
	}
	if c.Scheduler.PromptTimeout.Duration() <= 0 {
		return fmt.Errorf("scheduler.prompt_timeout must be positive")
	}
	wr := strings.TrimSpace(c.Scheduler.WorkspaceRetention)
	if wr == "" {
		c.Scheduler.WorkspaceRetention = "delete_on_session_close"
		wr = c.Scheduler.WorkspaceRetention
	}
	switch wr {
	case "delete_on_session_close", "archive_on_session_close":
	default:
		return fmt.Errorf("scheduler.workspace_retention must be delete_on_session_close or archive_on_session_close")
	}
	if len(c.Providers.Profiles) == 0 {
		return fmt.Errorf("providers.profiles must not be empty")
	}
	if _, err := c.DefaultProviderProfile(); err != nil {
		return err
	}
	for name, p := range c.Providers.Profiles {
		t := strings.TrimSpace(p.Transport)
		if t == "" {
			t = "acp"
			p.Transport = t
		}
		switch t {
		case "acp", "cursor_cli", "codex_app_server":
		default:
			return fmt.Errorf("providers.profiles.%s.transport must be acp, cursor_cli, or codex_app_server", name)
		}
		if strings.TrimSpace(p.Command) == "" {
			return fmt.Errorf("providers.profiles.%s.command is required", name)
		}
		c.Providers.Profiles[name] = p
	}
	lvl := strings.ToLower(strings.TrimSpace(c.Logging.Level))
	switch lvl {
	case "", "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("logging.level must be one of debug, info, warn, error")
	}
	return nil
}
