package slackapp

import "testing"

func TestPreviewRunes(t *testing.T) {
	if got := previewRunes("hello", 10); got != "hello" {
		t.Fatal(got)
	}
	if got := previewRunes("αβγδ", 3); got != "αβγ…" {
		t.Fatal(got)
	}
}
