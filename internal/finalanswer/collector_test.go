package finalanswer

import (
	"encoding/json"
	"testing"
)

func TestCollectorAgentChunks(t *testing.T) {
	var c Collector
	raw := json.RawMessage(`{"sessionId":"s1","sessionUpdate":{"type":"agent_message_chunk","content":{"type":"text","text":"hel"}}}`)
	c.OnSessionUpdateJSON(raw)
	raw2 := json.RawMessage(`{"sessionId":"s1","sessionUpdate":{"type":"agent_message_chunk","content":{"type":"text","text":"lo"}}}`)
	c.OnSessionUpdateJSON(raw2)
	if c.Text() != "hello" {
		t.Fatal(c.Text())
	}
}

func TestFallbackMessage(t *testing.T) {
	if FallbackMessage("end_turn") == "" {
		t.Fatal()
	}
}
