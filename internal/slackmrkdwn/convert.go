// Package slackmrkdwn converts common Markdown (as emitted by many agents) into Slack mrkdwn.
// Core conversion follows nextlevelbuilder/goclaw (see format.go).
package slackmrkdwn

import "strings"

// CommonMarkdownToMrkdwn maps GitHub/CommonMark-style constructs into Slack mrkdwn.
func CommonMarkdownToMrkdwn(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return markdownToSlackMrkdwn(s)
}
