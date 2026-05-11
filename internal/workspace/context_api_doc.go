package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteSlackContextAPIReference writes references/slack-context-api.md for Agent on-demand thread fetches.
// The bearer token is injected only via environment variables (see scheduler), not written to disk.
func WriteSlackContextAPIReference(workspaceDir, baseURL string) error {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return fmt.Errorf("context API base URL is empty")
	}
	refDir := filepath.Join(workspaceDir, "references")
	if err := os.MkdirAll(refDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(refDir, "slack-context-api.md")

	var b strings.Builder
	b.WriteString("# Slack 线程上下文 HTTP API（schduler）\n\n")
	b.WriteString("本会话工作区对应**一条** Slack 线程。为节省 token，请通过 schduler 暴露的 HTTP API **按需**拉取更早的消息，不要凭空臆测历史。\n\n")
	b.WriteString("## 环境变量\n\n")
	b.WriteString("- `SCHDULER_CONTEXT_API_URL` — 基础地址：")
	b.WriteString(baseURL)
	b.WriteString("\n")
	b.WriteString("- `SCHDULER_CONTEXT_API_TOKEN` — 本会话密钥（勿提交仓库）\n\n")
	b.WriteString("## 鉴权\n\n")
	b.WriteString("每个请求都带请求头 `Authorization: Bearer <token>`。该 token 仅对当前工作区会话有效。\n\n")
	b.WriteString("## 接口\n\n")
	b.WriteString("### `GET " + baseURL + "/v1/slack/thread/messages`\n\n")
	b.WriteString("查询参数：\n\n")
	b.WriteString("- `limit` — 最多返回条数（默认 50，最大 200）\n")
	b.WriteString("- `exclude_ts` — 跳过该 Slack `ts` 对应的消息（与入站用户消息元数据块里的 `trigger_message_ts` 对齐使用）\n")
	b.WriteString("- `oldest` — 仅返回该 Slack `ts` **之后**的消息（增量拉取）\n\n")
	b.WriteString("JSON 响应示例：\n\n```json\n{\n  \"channel_id\": \"C…\",\n  \"team_id\": \"T…\",\n  \"root_thread_ts\": \"…\",\n  \"messages\": [\n    {\"ts\":\"…\",\"user\":\"U…\",\"text\":\"…\",\"subtype\":\"\"}\n  ]\n}\n```\n\n")
	b.WriteString("## 示例\n\n```bash\ncurl -sS -H \"Authorization: Bearer $SCHDULER_CONTEXT_API_TOKEN\" \\\n  \"$SCHDULER_CONTEXT_API_URL/v1/slack/thread/messages?limit=30&exclude_ts=$TRIGGER_TS\"\n```\n\n")
	b.WriteString("### `GET " + baseURL + "/healthz`\n\n")
	b.WriteString("服务正常时返回 `ok`。\n")

	return os.WriteFile(path, []byte(b.String()), 0o644)
}
