package finalanswer

import (
	"encoding/json"
	"strings"
)

// Collector gathers assistant-visible text from session/update notifications.
type Collector struct {
	assistant strings.Builder
}

// OnSessionUpdateJSON parses a session/update notification params payload
// (the full JSON-RPC params object: sessionId + update).
func (c *Collector) OnSessionUpdateJSON(params json.RawMessage) {
	var notif struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"sessionUpdate"`
		Alt       json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(params, &notif); err != nil {
		return
	}
	u := notif.Update
	if len(u) == 0 {
		u = notif.Alt
	}
	c.ingestUpdate(u)
}

func (c *Collector) ingestUpdate(raw json.RawMessage) {
	var disc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &disc); err != nil {
		return
	}
	switch disc.Type {
	case "agent_message_chunk":
		var chunk struct {
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &chunk); err == nil && chunk.Content.Type == "text" {
			c.assistant.WriteString(chunk.Content.Text)
		}
	case "user_message_chunk", "agent_thought_chunk":
		// Ignore user streaming and hidden reasoning per product requirement.
	default:
		// tool_call and others ignored for Slack final reply.
	}
}

// Text returns accumulated assistant text.
func (c *Collector) Text() string {
	return c.assistant.String()
}

// Reset clears buffers for a new prompt turn.
func (c *Collector) Reset() {
	c.assistant.Reset()
}

// FallbackMessage produces a user-visible string when collector is empty.
func FallbackMessage(stopReason string) string {
	s := strings.TrimSpace(stopReason)
	if s == "" {
		return "The assistant had no text output for this turn."
	}
	return "Turn finished (" + s + ")."
}
