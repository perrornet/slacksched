# slacksched

Slack **Socket Mode** service that forwards each thread to a local **Agent Client Protocol (ACP)**-style or Cursor/Codex provider over stdio. One workspace and one provider process per Slack thread; only the **final** assistant reply is posted to the channel.

---

## Overview

| | |
|--|--|
| **Inbound** | Slack events → filtered, deduped, optional thread transcript |
| **Outbound** | `chat.postMessage` with Markdown → Slack mrkdwn conversion |
| **Providers** | `codex_app_server`, `cursor_cli`, or `acp` (see below) |

---

## Quick start

```bash
export SLACK_BOT_TOKEN=xoxb-...    # Bot User OAuth Token
export SLACK_APP_TOKEN=xapp-...    # App-level token (Socket Mode)

go run ./cmd/slacksched -config configs/example.yaml
```

Edit `configs/example.yaml`: set Slack allowlists (`allowed_dm_user_ids`, `allowed_channel_ids`, etc.) and provider command paths (`cursor-agent`, `codex`, …).

---

## Requirements

- **Go** 1.22+
- **Slack app** with Bot token and App-Level token (`connections:write` for Socket Mode)
- **Provider binary** on `PATH` matching your profile (`cursor-agent`, `codex`, …)

---

## Configuration

Default config path: [`configs/example.yaml`](configs/example.yaml). Override with `-config`.

### Logging (`logging`)

| Key | Meaning |
|-----|---------|
| `level` | `debug` / `info` / `warn` / `error` (default `info`) |
| `acp_trace` | Log every JSON-RPC line on provider stdio (truncated when huge) |
| `slack_trace` | Add short text previews to `slack_inbound` / `slack_outbound` logs |
| `file_path` | Also append logs to this file (optional) |

### Environment variables (secrets)

| Variable | Role |
|----------|------|
| `SLACK_BOT_TOKEN` | Bot OAuth token (`xoxb-…`) |
| `SLACK_APP_TOKEN` | Socket Mode app token (`xapp-…`) |

Names can be overridden in YAML: `slack.bot_token_env`, `slack.app_token_env`.

### Workspace and `AGENTS.md`

Each Slack thread gets its own workspace under `scheduler.workspaces_root`.

- `scheduler.agent_md_filename`: the generated instruction file name inside each workspace.
- `scheduler.agent_md_append_path`: optional Markdown file appended to generated `AGENTS.md` after scheduler-owned sections. Use this for stable global rules; see [docs/agent-extra.md](docs/agent-extra.md).
- `scheduler.slack_mrkdwn_guide_path`: optional file copied to `references/slack-mrkdwn-guide.md`.
- `scheduler.pre_session_command`: optional shell hook run after workspace creation and before the provider starts.

Generated `AGENTS.md` structure is:

1. scheduler-owned constraints
2. built-in session intro
3. runtime-updated Slack context block
4. optional bot identity section
5. optional Context HTTP API section
6. optional user appendix from `agent_md_append_path`

Do not use `agent_md_append_path` to replace runtime context, generated constraints, or the Context API usage section. Those are owned by the scheduler and may be refreshed between turns.

### Slack routing & threads

- **DMs**: only if the sender’s Slack user ID is listed in `slack.allowed_dm_user_ids`.
- **Channels**: optional `allowed_channel_ids`; if empty, channels are allowed subject to `require_mention_in_channels`.
- **Thread follow-ups**: once a session exists, further messages in the thread need no new mention.
- **`thread_replies_in_prompt`**: when `true`, loads `conversations.replies` and prepends a transcript (needs history scopes + reinstall).
- **`context_api_listen`**: when non-empty, starts a local read-only HTTP API so the agent can fetch current-thread context on demand.

Info-level logs: `slack_inbound`, `slack_outbound`, `slack_inbound_skipped` (filtered).

### Provider profiles (`providers.profiles`)

Each profile sets `transport`, `command`, optional `args`, `model`, `env`, `mode`.

| `transport` | Use case |
|---------------|----------|
| **`codex_app_server`** | Codex via `codex app-server --listen stdio://` (JSON-RPC thread/turn). Extra `args` only; duplicate `--listen` is stripped. |
| **`acp`** | Long-lived ACP over stdio (`session/new`, `session/prompt`). |
| **`cursor_cli`** | One-shot Cursor CLI: `cursor-agent chat … --output-format stream-json`. Core flags are built in; `args` are extras only. |

---

## Prebuilt binaries

Creating a **version tag** (e.g. `v1.0.0`) and pushing it runs [`.github/workflows/release.yml`](.github/workflows/release.yml): Linux and macOS **amd64** and **arm64** artifacts with **SHA256** checksums are attached to a **GitHub Release** for that tag.

```bash
git tag v1.0.0
git push origin v1.0.0
```

---

## Run (details)

```bash
go run ./cmd/slacksched -config configs/example.yaml
```

Build a static binary locally (example):

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o slacksched ./cmd/slacksched
```

---

## Development

```bash
go mod tidy
go test ./...
```

If checksum lookup hangs: `GOSUMDB=off GOPROXY=direct go mod tidy`.

If your shell exports a mismatched `GOROOT`, use:

```bash
env -u GOROOT go test ./...
```

---

## Project layout

| Path | Role |
|------|------|
| `cmd/slacksched` | Entrypoint |
| `internal/config` | YAML loading |
| `internal/slackapp` | Socket Mode, filters, `chat.postMessage` |
| `internal/scheduler` | Per-thread queue, workspace + provider lifecycle |
| `internal/provider` | ACP, Cursor CLI, Codex app-server |
| `internal/acp` | Newline-delimited JSON-RPC client |
| `internal/workspace` | Session dirs, `AGENTS.md` |
| `internal/messagefilter` | Policy + dedupe |
| `internal/finalanswer` | Final assistant text from streaming |
| `internal/session` | Slack thread key |
