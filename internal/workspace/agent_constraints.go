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
	b.WriteString("# 面向 ACP Provider 的会话工作区约束\n\n")
	b.WriteString("你在一个自动化的 Slack 调度器（schduler）里运行。\n\n")
	b.WriteString("- 除非用户明确要求，否则改动限定在**本会话工作区目录**内。\n")
	b.WriteString("- 不确定时，先提一个**很短的澄清问题**，不要随意猜测。\n")
	if strings.TrimSpace(contextAPIBaseURL) != "" {
		b.WriteString("- 当 shell 里已设置 `SCHDULER_CONTEXT_API_URL` 与 `SCHDULER_CONTEXT_API_TOKEN` 时，调度器在本机提供了 **Slack 线程上下文 HTTP API**。完整路径、curl 示例与允许调用的 Slack Web 方法列表已写入本文件 `")
		b.WriteString(agent)
		b.WriteString("` **文末**「Slack 线程上下文 HTTP API」一节（由程序生成，与当前调度器二进制一致）。按需拉取线程历史，不要凭空臆测；不要默认完整长上下文已随 Slack 入站消息一并提供。\n")
	} else {
		b.WriteString("- 本会话未启用按需 HTTP 上下文 API。不要假设可通过调度器暴露的本地 HTTP 接口拉取线程；需要历史时依赖 Slack 入站内容与本仓库内其它材料。\n")
	}
	return b.String()
}
