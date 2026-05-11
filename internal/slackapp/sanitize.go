package slackapp

import (
	"regexp"
	"strings"
)

func newSelfMentionRE(userID string) *regexp.Regexp {
	id := strings.TrimSpace(userID)
	if id == "" {
		return nil
	}
	return regexp.MustCompile(`<@` + regexp.QuoteMeta(id) + `(?:\|[^>]+)?>`)
}

func stripSelfMentions(re *regexp.Regexp, text string) string {
	if re == nil || text == "" {
		return text
	}
	return re.ReplaceAllString(text, "")
}
