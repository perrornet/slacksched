package slackassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// setStatusEndpoint is the Slack Web API URL; tests may replace it.
var setStatusEndpoint = "https://slack.com/api/assistant.threads.setStatus"

// MaxLoadingMessages is the Slack API limit for loading_messages.
const MaxLoadingMessages = 10

// ThreadStatus sets assistant.threads.setStatus (see
// https://api.slack.com/methods/assistant.threads.setStatus). Pass an
// empty Status to clear the indicator without posting a message.
func ThreadStatus(ctx context.Context, hc *http.Client, botToken string, p ThreadStatusParams) error {
	if hc == nil {
		hc = http.DefaultClient
	}
	channelID := strings.TrimSpace(p.ChannelID)
	threadTS := strings.TrimSpace(p.ThreadTS)
	if channelID == "" || threadTS == "" {
		return fmt.Errorf("slackassistant: channel_id and thread_ts are required")
	}
	msgs := p.LoadingMessages
	if len(msgs) > MaxLoadingMessages {
		msgs = msgs[:MaxLoadingMessages]
	}
	body := map[string]any{
		"channel_id": channelID,
		"thread_ts":  threadTS,
		"status":     p.Status, // may be "" to clear
	}
	if len(msgs) > 0 {
		body["loading_messages"] = msgs
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setStatusEndpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(botToken))

	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var api struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &api); err != nil {
		return fmt.Errorf("slackassistant: parse response: %w (body=%q)", err, truncate(string(b), 200))
	}
	if !api.Ok {
		e := strings.TrimSpace(api.Error)
		if e == "" {
			e = "unknown"
		}
		return fmt.Errorf("slackassistant: assistant.threads.setStatus: %s", e)
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// ThreadStatusParams are JSON fields for assistant.threads.setStatus.
type ThreadStatusParams struct {
	ChannelID        string
	ThreadTS         string
	Status           string
	LoadingMessages  []string
}
