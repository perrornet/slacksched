package workspace

import "strings"

// BuildSessionBootstrapMarkdown is the fixed first user turn on a new provider session (ACP has no
// system channel). The scheduler discards this turn's assistant text for Slack; only the following
// turn's reply is posted. The user's real Slack text is sent in that second turn.
func BuildSessionBootstrapMarkdown() string {
	return strings.TrimSpace(sessionBootstrapMarkdown)
}

const sessionBootstrapMarkdown = `
你是正在 schduler 工作区中运行的本地开发 Agent。

细则见本会话工作区根目录的 AGENTS.md（文首为会话约束；若启用了 Slack 上下文 HTTP API，用法在文末）。

请先阅读 AGENTS.md。

若需要本 Slack 线程中更早的消息，且运行环境已设置 SCHDULER_CONTEXT_API_URL 与 SCHDULER_CONTEXT_API_TOKEN，按 AGENTS.md 中的 HTTP API 说明获取。不要假定整条线程已随单条入站消息一并送达。
`
