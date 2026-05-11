package workspace

import (
	"strings"

	"github.com/perrornet/slacksched/internal/contextapi"
)

// BuildAgentContextAPISectionMarkdown returns markdown documenting the local Slack context HTTP API,
// wired to the actual base URL for this session (must match SCHDULER_CONTEXT_API_URL in the provider env).
// Empty baseURL yields empty string.
func BuildAgentContextAPISectionMarkdown(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimSuffix(baseURL, "/")
	if baseURL == "" {
		return ""
	}
	methods := contextapi.AllowedWebAPIMethods()
	methodsQuoted := make([]string, len(methods))
	for i, m := range methods {
		methodsQuoted[i] = "`" + m + "`"
	}
	methodsJoin := strings.Join(methodsQuoted, "、")

	var b strings.Builder
	b.WriteString("\n## Slack 线程上下文 HTTP API\n\n")
	b.WriteString("以下为调度器根据当前配置**自动生成**，与运行的 HTTP 处理器一致；请勿依赖手写副本。\n\n")
	b.WriteString("shell 中由进程注入：`SCHDULER_CONTEXT_API_URL`、`SCHDULER_CONTEXT_API_TOKEN`。本会话基础地址：\n\n")
	b.WriteString("```\n")
	b.WriteString(baseURL)
	b.WriteString("\n```\n\n")
	b.WriteString("所有请求使用请求头 `Authorization: Bearer <SCHDULER_CONTEXT_API_TOKEN>`。**不要**在请求体中传入 Slack bot `token`（由服务端注入）。\n\n")
	b.WriteString("### 摘要接口（可选）\n\n")
	b.WriteString("`GET " + baseURL + "/v1/slack/thread/messages`\n\n")
	b.WriteString("查询参数：`limit`（默认 50，最大 200）、`exclude_ts`（与入站元数据 `trigger_message_ts` 对齐）、`oldest`（仅返回该 `ts` 之后的消息）。\n\n")
	b.WriteString("```bash\ncurl -sS -H \"Authorization: Bearer $SCHDULER_CONTEXT_API_TOKEN\" \\\n  \"" + baseURL + "/v1/slack/thread/messages?limit=30&exclude_ts=$TRIGGER_MESSAGE_TS\"\n```\n\n")
	b.WriteString("### 只读 Slack Web API 代理（完整消息结构）\n\n")
	b.WriteString("`POST " + baseURL + "/v1/slack/web-api/<方法名>`\n\n")
	b.WriteString("正文：`application/json`（推荐）或 `application/x-www-form-urlencoded`。响应与 Slack 官方 Web API 一致（含 `blocks`、`attachments` 等）。\n\n")
	b.WriteString("当前允许的方法：" + methodsJoin + "。\n\n")
	b.WriteString("本会话约束：\n\n")
	b.WriteString("- `conversations.replies`：`channel` 须为入站元数据中的 `channel_id`；`ts` 须为 **`root_thread_ts`**。\n")
	b.WriteString("- `conversations.info`、`conversations.history`：`channel` 须为该 `channel_id`。\n")
	b.WriteString("- `team.info`：若提供 `team`，须为入站 `team_id`。\n\n")
	b.WriteString("```bash\ncurl -sS -X POST -H \"Authorization: Bearer $SCHDULER_CONTEXT_API_TOKEN\" \\\n  -H \"Content-Type: application/json\" \\\n  -d \"{\\\"channel\\\":\\\"$CHANNEL_ID\\\",\\\"ts\\\":\\\"$ROOT_THREAD_TS\\\",\\\"limit\\\":200}\" \\\n  \"" + baseURL + "/v1/slack/web-api/conversations.replies\"\n```\n\n")
	b.WriteString("### 健康检查\n\n")
	b.WriteString("`GET " + baseURL + "/healthz`\n\n")
	b.WriteString("正常时响应正文为 `ok`。\n")
	return b.String()
}
