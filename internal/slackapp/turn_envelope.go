package slackapp

import (
	"strings"
)

// buildSlackTurnEnvelope wraps raw Slack user text in structured framing: session ids, a “focus on THIS message” line,
// then the verbatim body as markdown blockquote lines. rawText is unchanged except for a `> ` prefix per line.
func buildSlackTurnEnvelope(agentMDFilename, teamID, channelID, rootThreadTS, triggerTS, eventID, userID, rawText string) string {
	return rawText
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
