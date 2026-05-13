package workspace

import "strings"

// BuiltinAgentMarkdownTemplate is the first provider user turn (scheduler sessionBootstrapBody).
// Placeholders are filled by BuildSessionOpeningPrompt; it is not written verbatim to AGENTS.md
// (see BuiltinAgentMarkdownFileIntro for the on-disk section).
const BuiltinAgentMarkdownTemplate = `
你在为本 Slack 会话工作；请先阅读同目录下的 AGENTS.md再作答。

- 频道 ID：<slack-channel-id>
- 频道名称：<slack-channel-name>
- 用户 ID：<mention-user-id>

**用户本条消息**：

<mention-user-message>
`

// BuiltinAgentMarkdownFileIntro is embedded in each new session AGENTS.md after the constraints
// block. It has no placeholders; runtime-filled opening text is sessionBootstrapBody only.
const BuiltinAgentMarkdownFileIntro = `## 会话说明

你在为本 Slack 会话工作。环境与线程信息见下方「Slack 会话上下文」（随消息刷新）；操作范围见「当前会话约束」。**首条具体任务与频道快照由调度器在会话第一条 provider 消息中发送**，请以该条为准。
`

// BuildSessionOpeningPrompt fills BuiltinAgentMarkdownTemplate for the first provider prompt.
// userMessage is normally scheduler Job.Text (e.g. turn-enveloped).
//
// Placeholders: <slack-channel-id>, <slack-channel-name>, <mention-user-id>, <mention-user-message>.
// If the template body omits <mention-user-message>, userMessage is appended after the body.
func BuildSessionOpeningPrompt(sc SlackRuntimeContext, userID, userMessage string) string {
	return buildSessionOpeningFromTemplate(BuiltinAgentMarkdownTemplate, sc, userID, userMessage)
}

func buildSessionOpeningFromTemplate(raw string, sc SlackRuntimeContext, userID, userMessage string) string {
	chName := strings.TrimSpace(sc.ChannelName)
	if chName == "" {
		if sc.IsIM {
			chName = "（私信）"
		} else {
			chName = "（未知频道名）"
		}
	}
	uid := strings.TrimSpace(userID)
	msg := strings.TrimRight(userMessage, "\n\r")
	repl := strings.NewReplacer(
		"<slack-channel-id>", strings.TrimSpace(sc.ChannelID),
		"<slack-channel-name>", chName,
		"<mention-user-id>", uid,
		"<mention-user-message>", msg,
	)
	out := strings.TrimSpace(repl.Replace(raw))
	if !strings.Contains(raw, "<mention-user-message>") {
		if out != "" {
			out = out + "\n\n" + msg
		} else {
			out = msg
		}
	}
	return out
}
