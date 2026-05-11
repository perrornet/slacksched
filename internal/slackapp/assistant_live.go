package slackapp

import (
	"strings"

	"github.com/perrornet/slacksched/internal/config"
	"github.com/perrornet/slacksched/internal/slackassistant"
)

// defaultAssistantStatusSuffix is the base API "status" line: Slack renders it as
// "<app name> is <suffix>". When tools were used, buildLiveStatusLine appends the tool list.
func defaultAssistantStatusSuffix(slack *config.SlackConfig) string {
	if slack == nil {
		return "Working on your request…"
	}
	s := strings.TrimSpace(slack.AssistantStatusText)
	if s == "" {
		return "Working on your request…"
	}
	return s
}

// buildLiveStatusLine combines the stable suffix with a deduped, ordered list of tool ids
// invoked during this Slack turn (e.g. "Working on your request… — tools: glob, read, shell").
func buildLiveStatusLine(base string, toolsUsed []string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "Working on your request…"
	}
	if len(toolsUsed) == 0 {
		return base
	}
	return base + " — tools: " + strings.Join(toolsUsed, ", ")
}

// carouselToolPhrase maps stream tool ids (Cursor *.json keys minus "ToolCall", Codex kinds, MCP names)
// to a one-line English description. Unknown ids fall back to "Calling <id>…" so the UI stays tied to what ran.
func carouselToolPhrase(toolID string) string {
	switch toolID {
	case "shell":
		return "Running a shell command…"
	case "read":
		return "Reading a file…"
	case "write":
		return "Writing a file…"
	case "glob":
		return "Glob-matching files in the workspace…"
	case "patch":
		return "Applying file edits…"
	case "search":
		return "Searching the web…"
	case "mcp":
		return "Calling an MCP tool…"
	case "tool":
		return "Calling a tool…"
	default:
		return ""
	}
}

func streamPhaseCarouselDetail(phase, tool string) string {
	tool = strings.TrimSpace(tool)
	switch phase {
	case "thinking":
		return "Thinking…"
	case "tool_call":
		if tool == "" {
			return "Calling a tool…"
		}
		if s := carouselToolPhrase(tool); s != "" {
			return s
		}
		return "Calling " + tool + "…"
	case "idle":
		return "Finishing up…"
	default:
		if tool != "" {
			if s := carouselToolPhrase(tool); s != "" {
				return s
			}
			return "Calling " + tool + "…"
		}
		return "Working…"
	}
}

// buildAssistantLoadingCarousel returns up to 10 lines for assistant.threads.setStatus
// "loading_messages" — Slack rotates these (gold / shimmer loading strip). See
// https://api.slack.com/methods/assistant.threads.setStatus
//
// phase: "bootstrap" (before stream), "thinking", "tool_call", "idle", or "".
func buildAssistantLoadingCarousel(phase, tool string) []string {
	var lines []string
	switch phase {
	case "bootstrap":
		lines = []string{
			"Message received…",
			"Finding context…",
			"Planning next steps…",
		}
	case "thinking":
		detail := streamPhaseCarouselDetail(phase, tool)
		if detail == "" {
			detail = "Thinking…"
		}
		lines = []string{
			detail,
			"Finding relevant context…",
			"Gathering details…",
		}
	case "tool_call":
		detail := streamPhaseCarouselDetail(phase, tool)
		if detail == "" {
			detail = "Calling a tool…"
		}
		lines = []string{
			detail,
			"Running tools and collecting results…",
		}
	case "idle":
		lines = []string{
			"Almost done…",
		}
	default:
		detail := streamPhaseCarouselDetail(phase, tool)
		if detail == "" {
			detail = "Working…"
		}
		lines = []string{detail, "Please wait…", "Still working…"}
	}
	if len(lines) > slackassistant.MaxLoadingMessages {
		return lines[:slackassistant.MaxLoadingMessages]
	}
	return lines
}
