package slackapp

import "unicode/utf8"

const maxSlackTextPreviewRunes = 512

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func previewRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i] + "…"
		}
		n++
	}
	return s
}
