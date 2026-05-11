# schduler

Go service that connects Slack (Socket Mode) to local **Agent Client Protocol (ACP)** providers (for example Codex or Cursor) over newline-delimited JSON-RPC on stdio. Each new Slack thread gets its own workspace directory, `AGENTS.md`, and **one dedicated provider process**. Only the final assistant reply is posted back to Slack; `session/update` streaming is absorbed in memory.

## Prerequisites

- Go 1.22+
- Slack app with **Bot Token** and **App-Level Token** (connections:write) for Socket Mode
- A locally installed provider binary (e.g. `codex`, `cursor-agent`) matching `configs/example.yaml`

## Configuration

- YAML path defaults to `configs/example.yaml` (`-config` flag).

### Logging

- `logging.level`: `debug`, `info`, `warn`, or `error` (default `info` when omitted).
- `logging.acp_trace`: when `true`, every newline-delimited JSON-RPC message on the provider stdio is logged at **Info** with key `acp_trace` and attributes `direction=send|recv` and `raw=<line>`. Lines longer than ~256KiB are truncated for safety. This is independent of what gets posted to Slack (Slack still only sees the final reply).
- `logging.slack_trace`: when `true`, `slack_inbound` and `slack_outbound` logs include a short `text_preview` / `outbound_text_preview` (trimmed to a few hundred runes). When `false`, only metadata and lengths are logged.
- `logging.file_path`: when non-empty, the same text-format logs are **also** appended to this file (stdout remains enabled). Parent directories are created if missing. Leave empty to log only to the terminal.

**Slack flow logs** (at Info): `slack_inbound` when a user message is accepted and enqueued; `slack_outbound` after `chat.postMessage` (includes `posted_ts` on success, or `err` on failure); `slack_inbound_skipped` when the filter rejects an event.

### Environment variables

| Variable            | Purpose                                      |
|---------------------|----------------------------------------------|
| `SLACK_BOT_TOKEN`   | Bot OAuth token (`xoxb-...`)                 |
| `SLACK_APP_TOKEN`   | App-level token for Socket Mode (`xapp-...`) |

Token **names** can be overridden in YAML via `slack.bot_token_env` and `slack.app_token_env`.

### Slack behaviour

- **DMs** only from users listed in `slack.allowed_dm_user_ids`.
- **Channels** restricted when `allowed_channel_ids` is non-empty; if empty, all channels are allowed subject to `require_mention_in_channels`.
- Once a thread has an active session, further messages in that thread are accepted without another mention.
- With **`slack.thread_replies_in_prompt: true`**, the service calls [`conversations.replies`](https://api.slack.com/methods/conversations.replies) for the thread root (`root_thread_ts`), prepends a short transcript (excluding the triggering message) to the provider prompt, then appends `Message to answer:` and the current user text. Grant history scopes (`channels:history`, `groups:history`, `im:history` as needed) and reinstall the app so the bot can read thread messages.
- Retries are deduped using `event_id` and `client_msg_id` (short TTL).

### Provider profiles

`providers.profiles` defines `transport`, `command`, `args`, optional `model`, `env`, and optional `mode` label.

- **`transport: codex_app_server`** (recommended for Codex): runs [`codex app-server --listen stdio://`](https://developers.openai.com/codex/app-server) with JSON-RPC `initialize` → `thread/start` → per-message `turn/start`, matching [multica-ai/multica `codex.go`](https://github.com/multica-ai/multica/blob/main/server/pkg/agent/codex.go). `args` are **extra** flags only; `--listen stdio://` is always injected; a duplicate `--listen` in `args` is stripped.
- **`transport: acp`**: long-lived stdio [**Agent Client Protocol**](https://agentclientprotocol.com/) (`session/new`, `session/prompt`). Use for agents that speak ACP, not for the interactive `codex acp` TUI.
- **`transport: cursor_cli`**: [Cursor CLI](https://cursor.com/docs/cli/headless) one-shot per message: `cursor-agent chat -p "…" --output-format stream-json --yolo …`, same argv shape as [multica `cursor.go`](https://github.com/multica-ai/multica/blob/main/server/pkg/agent/cursor.go). Core flags are built in code; `args` are filtered extras only. Optional `model` sets `--model`.

### Troubleshooting: `stdin is not a terminal` (stderr)

The scheduler talks to the provider over **pipes** (stdin/stdout are not a TTY). Some CLI entrypoints still call `isatty(stdin)` and abort if they think they are not in an interactive terminal.

- This is **not** a bug in the Slack side; fix the **provider entrypoint**.
- For **Codex**, prefer **`transport: codex_app_server`** (`codex app-server --listen stdio://`) instead of an interactive subcommand such as `codex acp` that expects a terminal.
- For **ACP-only agents**, use **`transport: acp`** with a binary that implements **ACP** over stdio.
- For **Cursor**, use **`transport: cursor_cli`** (`cursor-agent chat -p …`); do not use a mode that expects a TTY on stdin.

## Run

```bash
export SLACK_BOT_TOKEN=xoxb-...
export SLACK_APP_TOKEN=xapp-...
go run ./cmd/schduler -config configs/example.yaml
```

## Tests

```bash
go mod tidy
go test ./...
```

If module checksum lookup hangs, try `GOSUMDB=off GOPROXY=direct go mod tidy`.

## Layout

- `cmd/schduler` — entrypoint
- `internal/config` — YAML + validation
- `internal/slackapp` — Socket Mode, filtering, final `chat.postMessage`
- `internal/scheduler` — per-thread FIFO, idle timeout, workspace + provider lifecycle
- `internal/provider` — ACP stdio, **Cursor CLI** (`cursor_cli`), **Codex app-server** (`codex_app_server`)
- `internal/acp` — JSON-RPC newline client
- `internal/workspace` — session directories and `AGENTS.md`
- `internal/messagefilter` — Slack policy + dedupe
- `internal/finalanswer` — assistant text from `session/update`
- `internal/session` — Slack thread key type
