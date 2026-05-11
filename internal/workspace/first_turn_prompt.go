package workspace

import "strings"

// ComposeFirstTurnPrompt prepends optional markdown (from first_turn_prompt_md_path) to the
// first provider user turn only. Caller clears the pending flag after a successful Prompt.
func ComposeFirstTurnPrompt(prefixMD, slackPayload string) string {
	pre := strings.TrimSpace(prefixMD)
	slackPayload = strings.TrimRight(slackPayload, "\n")
	if pre == "" {
		return slackPayload
	}
	if slackPayload == "" {
		return pre
	}
	return pre + "\n\n---\n\n" + slackPayload
}
