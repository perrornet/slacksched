package slackapp

import (
	"fmt"
	"strings"
)

// buildSlackTurnEnvelope wraps raw Slack user text in structured framing: session ids, a “focus on THIS message” line,
// then the verbatim body as markdown blockquote lines. rawText is unchanged except for a `> ` prefix per line.
func buildSlackTurnEnvelope(agentMDFilename, teamID, channelID, rootThreadTS, triggerTS, eventID, userID, rawText string) string {
	rawText = strings.TrimRight(rawText, "\n")
	doc := normalizedAgentDocName(agentMDFilename)
	var b strings.Builder
	b.WriteString("你是通过 schduler 在 Slack 线程中接入的本地编码 Agent\n\n")
	b.WriteString("本会话定位字段：\n")
	b.WriteString(fmt.Sprintf("- `team_id`: `%s`\n", strings.TrimSpace(teamID)))
	b.WriteString(fmt.Sprintf("- `channel_id`: `%s`\n", strings.TrimSpace(channelID)))
	b.WriteString(fmt.Sprintf("- `root_thread_ts`: `%s`\n", strings.TrimSpace(rootThreadTS)))
	b.WriteString(fmt.Sprintf("- `trigger_message_ts`: `%s`（本条待响应的 Slack 消息）\n", strings.TrimSpace(triggerTS)))
	b.WriteString(fmt.Sprintf("- `event_id`: `%s`\n", strings.TrimSpace(eventID)))
	if u := strings.TrimSpace(userID); u != "" {
		b.WriteString(fmt.Sprintf("- `user_id`: `%s`\n", u))
	}
	b.WriteString("\n[新消息] 用户刚发送了下面内容。请只针对 **本条** 理解与行动，不要与线程里更早的消息混淆。\n\n")
	b.WriteString(markdownBlockquotePerLine(rawText))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("频道 / 线程的补充说明与（可选）历史摘录见工作区 `%s` 中「Slack 会话上下文」一节；该块会在每条入站前刷新。\n", doc))
	b.WriteString("面向用户的可见回复应发回当前 Slack 线程；不要在回复中泄露 Context API token 等机密。\n")
	b.WriteString("不要用字面量 `\\n` 转义串冒充换行；引用块内即为用户原文的真实换行。")
	return b.String()
}

func normalizedAgentDocName(agentMDFilename string) string {
	s := strings.TrimSpace(agentMDFilename)
	if s == "" {
		return "AGENTS.md"
	}
	return s
}

func markdownBlockquotePerLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return "> "
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("> ")
		b.WriteString(line)
	}
	return b.String()
}
