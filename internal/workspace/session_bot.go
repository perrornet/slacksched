package workspace

import "strings"

// SessionBotIdentity is embedded once in AGENTS.md when a session workspace is created.
type SessionBotIdentity struct {
	// UserID is the bot member id (U…); mrkdwn user mention: <@UserID>.
	UserID string
	// BotID is the Slack app bot id (B…) when present.
	BotID string
	// UserName is the Slack username / handle (auth.test `user`).
	UserName string
	// DisplayName is a human-facing name from users.profile when available.
	DisplayName string
}

// agentMarkdownSection returns markdown to append to AGENTS.md, or empty if UserID is unset.
func (s SessionBotIdentity) agentMarkdownSection() string {
	uid := strings.TrimSpace(s.UserID)
	if uid == "" {
		return ""
	}
	display := strings.TrimSpace(s.DisplayName)
	if display == "" {
		display = strings.TrimSpace(s.UserName)
	}
	var b strings.Builder
	b.WriteString("\n## 本会话的 Slack 机器人身份\n\n")
	b.WriteString("最终回复会以**本机器人**身份发到 Slack；解读线程里的 @ 或自我提及时可对照下列字段。\n\n")
	b.WriteString("- **bot_user_id**：`")
	b.WriteString(uid)
	b.WriteString("` — mrkdwn 提及形式：`<@")
	b.WriteString(uid)
	b.WriteString(">`\n")
	if display != "" {
		b.WriteString("- **bot_display_name**：")
		b.WriteString(display)
		b.WriteString("\n")
	}
	return b.String()
}
