package slackthread

import (
	"testing"

	"github.com/slack-go/slack"
)

func TestFormatLine(t *testing.T) {
	m := slack.Message{
		Msg: slack.Msg{
			Timestamp: "1.0",
			User:      "U1",
			Text:      "hi",
		},
	}
	if FormatLine(&m) != "[时间戳 1.0] U1: hi" {
		t.Fatalf("%q", FormatLine(&m))
	}
}

func TestFormatLine_Bot(t *testing.T) {
	m := slack.Message{
		Msg: slack.Msg{
			Timestamp: "2.0",
			BotID:     "B1",
			Text:      "beep",
		},
	}
	s := FormatLine(&m)
	if s != "[时间戳 2.0] 机器人:B1: beep" {
		t.Fatalf("%q", s)
	}
}
