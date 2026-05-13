package workspace

import "strings"

// BuildSchedulerAgentConstraintsMarkdown is the opening section of the generated AGENTS.md
// (Slack session constraints + pointer to the auto-generated API section at EOF).
func BuildSchedulerAgentConstraintsMarkdown(agentMDFilename, contextAPIBaseURL string) string {
	agent := strings.TrimSpace(agentMDFilename)
	if agent == "" {
		agent = "AGENTS.md"
	}

	var b strings.Builder
	b.WriteString("# 当前会话约束\n\n")
	b.WriteString("- 默认只在本会话工作区目录内改文件；不确定时先一句澄清。\n")
	b.WriteString("- 线程元数据见「Slack 会话上下文」；勿默认单条入站消息已含完整线程。\n")
	if strings.TrimSpace(contextAPIBaseURL) != "" {
		b.WriteString("- 若 shell 已设置 `SCHDULER_CONTEXT_API_URL` 与 `SCHDULER_CONTEXT_API_TOKEN`，可按本文件 `")
		b.WriteString(agent)
		b.WriteString("` 文末「Slack 线程上下文 HTTP API」按需拉历史。\n")
	} else {
		b.WriteString("- 本会话未启用按需 HTTP 上下文 API。\n")
	}
	return b.String()
}
