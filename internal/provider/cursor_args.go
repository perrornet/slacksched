// Cursor CLI argv assembly: core flags are built in; args are extras only.
// Typical shape: chat -p <prompt> --output-format stream-json --yolo [--workspace] [--model] [--resume], then filtered user extras.

package provider

import (
	"log/slog"
	"regexp"
	"strings"
)

// cursorBlockedArgs are flags that must not appear in YAML "args" overrides; they are set by the scheduler.
var cursorBlockedArgs = map[string]struct{}{
	"-p":              {},
	"--print":         {},
	"--output-format": {},
	"--yolo":          {},
	"--workspace":     {},
	"--resume":        {},
	"--model":         {},
	"chat":            {},
}

// normalizeCursorStreamLine strips optional stdout:/stderr: prefixes that Cursor CLI may emit.
var cursorStreamPrefixRe = regexp.MustCompile(`^(?i)(stdout|stderr)\s*[:=]?\s*`)

func normalizeCursorStreamLine(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if idx := cursorStreamPrefixRe.FindStringIndex(trimmed); idx != nil {
		return strings.TrimSpace(trimmed[idx[1]:])
	}
	return trimmed
}

func filterCursorExtraArgs(extra []string, log *slog.Logger) []string {
	if len(extra) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(extra); i++ {
		tok := extra[i]
		if _, bad := cursorBlockedArgs[tok]; bad {
			if log != nil {
				log.Warn("cursor_cli: ignoring blocked arg from config", "arg", tok)
			}
			if requiresCursorArgValue(tok) {
				if i+1 < len(extra) {
					i++
				}
			}
			continue
		}
		out = append(out, tok)
	}
	return out
}

func requiresCursorArgValue(flag string) bool {
	switch flag {
	case "--output-format", "--workspace", "--resume", "--model", "-p", "--print":
		return true
	default:
		return false
	}
}

func buildCursorCLIArgs(prompt, workspace, resume, model string, extra []string, log *slog.Logger) []string {
	args := []string{
		"chat",
		"-p", prompt,
		"--output-format", "stream-json",
		"--yolo",
	}
	if strings.TrimSpace(workspace) != "" {
		args = append(args, "--workspace", workspace)
	}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	if strings.TrimSpace(resume) != "" {
		args = append(args, "--resume", resume)
	}
	args = append(args, filterCursorExtraArgs(extra, log)...)
	return args
}
