package workspace

import (
	"fmt"
	"os"
	"strings"
)

// Markers delimit the block in AGENTS.md that ReplaceSlackContextBody overwrites.
const (
	slackContextStartMarker = "<!-- schduler-slack-context:start -->"
	slackContextEndMarker   = "<!-- schduler-slack-context:end -->"
)

// SlackRuntimeContext is refreshed by the scheduler before each provider prompt;
// user message text is not modified — only this block in AGENTS.md changes.
type SlackRuntimeContext struct {
	AgentDoc string // workspace-relative agent filename (e.g. AGENTS.md)

	TeamID            string
	ChannelID         string
	ChannelName       string // conversations.info name when not IM; may be empty on API failure
	IsIM              bool
	RootThreadTS      string
	TriggerMessageTS  string
	ContextAPIBaseURL string // non-empty when local context API is enabled for this worker

	// ThreadPriorTranscript is formatted lines (e.g. from slackthread.TranscriptExcluding), excluding the trigger message.
	ThreadPriorTranscript string
}

// BuildMarkdownBody returns the inner markdown for the delimited region (no markers).
func (c SlackRuntimeContext) BuildMarkdownBody() string {
	doc := strings.TrimSpace(c.AgentDoc)
	if doc == "" {
		doc = "AGENTS.md"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- `team_id`: `%s`\n", strings.TrimSpace(c.TeamID)))
	b.WriteString(fmt.Sprintf("- `channel_id`: `%s`\n", strings.TrimSpace(c.ChannelID)))
	if c.IsIM {
		b.WriteString("- 会话类型：Slack 私信（im）\n")
	} else {
		name := strings.TrimSpace(c.ChannelName)
		if name != "" {
			b.WriteString(fmt.Sprintf("- 频道名称：`#%s`\n", strings.TrimPrefix(name, "#")))
		} else {
			b.WriteString("- 频道名称：（未能从 Slack API 解析）\n")
		}
	}
	b.WriteString(fmt.Sprintf("- `root_thread_ts`: `%s`\n", strings.TrimSpace(c.RootThreadTS)))
	b.WriteString(fmt.Sprintf("- `trigger_message_ts`: `%s`（本条用户消息的 ts）\n", strings.TrimSpace(c.TriggerMessageTS)))
	if api := strings.TrimSpace(c.ContextAPIBaseURL); api != "" {
		b.WriteString(fmt.Sprintf("- Context HTTP API：`%s`（用法见 `%s` 文末）\n", api, doc))
	}
	ts := strings.TrimSpace(c.ThreadPriorTranscript)
	if ts != "" {
		b.WriteString("\n### 线程内先前消息（不含当前待答消息）\n\n")
		b.WriteString("```text\n")
		b.WriteString(ts)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// SlackContextSectionHTMLComment inserts the delimited region into a new AGENTS.md.
func SlackContextSectionHTMLComment(initialInner string) string {
	inner := strings.TrimRight(initialInner, "\n")
	if inner == "" {
		inner = "_（尚无具体上下文，首条入站后将刷新）_"
	}
	var b strings.Builder
	b.WriteString("\n## Slack 会话上下文（调度器自动更新）\n\n")
	b.WriteString("以下块随每条入站 Slack 消息刷新。**用户消息正文**不经修改、原样传给 Provider。\n\n")
	b.WriteString(slackContextStartMarker)
	b.WriteString("\n")
	b.WriteString(inner)
	b.WriteString("\n")
	b.WriteString(slackContextEndMarker)
	b.WriteString("\n\n")
	return b.String()
}

// ReplaceSlackContextBody overwrites the markdown between the start/end markers in agentPath.
func ReplaceSlackContextBody(agentPath, newInner string) error {
	data, err := os.ReadFile(agentPath)
	if err != nil {
		return fmt.Errorf("read agent md: %w", err)
	}
	s := string(data)
	i := strings.Index(s, slackContextStartMarker)
	j := strings.Index(s, slackContextEndMarker)
	if i < 0 || j < 0 || j <= i {
		return fmt.Errorf("%q: slack context markers not found", agentPath)
	}
	lineEnd := strings.Index(s[i+len(slackContextStartMarker):], "\n")
	if lineEnd < 0 {
		return fmt.Errorf("%q: malformed slack context start marker", agentPath)
	}
	contentStart := i + len(slackContextStartMarker) + lineEnd + 1
	inner := strings.TrimRight(newInner, "\n")
	replaced := s[:contentStart] + inner + "\n" + s[j:]
	return os.WriteFile(agentPath, []byte(replaced), 0o644)
}
