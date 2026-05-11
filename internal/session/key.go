package session

import "fmt"

// Key identifies a Slack conversation thread for scheduler binding.
type Key struct {
	TeamID         string
	ChannelID      string
	RootThreadTS   string
}

// String returns a stable map key.
func (k Key) String() string {
	return fmt.Sprintf("%s/%s/%s", k.TeamID, k.ChannelID, k.RootThreadTS)
}

// RootThread returns thread_ts for top-level messages, else parent thread.
func RootThread(threadTS, messageTS string) string {
	if threadTS != "" {
		return threadTS
	}
	return messageTS
}
