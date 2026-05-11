package provider

import (
	"context"
	"strings"
	"testing"
)

func TestParseCursorResultLine(t *testing.T) {
	line := `{"type":"result","subtype":"success","duration_ms":1,"is_error":false,"result":"hello","session_id":"abc-uuid","request_id":"x"}`
	text, sid, hit, err := parseCursorResultLine([]byte(line))
	if err != nil || !hit {
		t.Fatalf("hit=%v err=%v", hit, err)
	}
	if text != "hello" || sid != "abc-uuid" {
		t.Fatalf("got %q %q", text, sid)
	}
}

func TestParseCursorResultLineIgnoreAssistant(t *testing.T) {
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"x"}]},"session_id":"s"}`
	_, _, hit, err := parseCursorResultLine([]byte(line))
	if err != nil || hit {
		t.Fatalf("hit=%v err=%v", hit, err)
	}
}

func TestParseCursorToolCallKind(t *testing.T) {
	line := `{"type":"tool_call","subtype":"started","call_id":"x","tool_call":{"readToolCall":{"args":{"path":"/p"}}}}`
	if got := parseCursorToolCallKind([]byte(line)); got != "read" {
		t.Fatalf("got %q", got)
	}
}

func TestReadCursorStreamJSON_UsesLastAssistantNotConcatenatedResult(t *testing.T) {
	stream := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Checking docs.\n\n"}]},"session_id":"s"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Final reply only."}]},"session_id":"s"}
{"type":"result","subtype":"success","is_error":false,"result":"Checking docs.\n\nFinal reply only.","session_id":"s"}
`
	final, sid, _, err := readCursorStreamJSON(context.Background(), strings.NewReader(stream), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if final != "Final reply only." {
		t.Fatalf("got %q want last assistant only", final)
	}
	if sid != "s" {
		t.Fatalf("session id %q", sid)
	}
}

type streamEv struct {
	phase, tool string
}

func TestReadCursorStreamJSON_StreamPhases(t *testing.T) {
	var got []streamEv
	ctx := ContextWithStreamPhaseCallback(context.Background(), func(p, tool string) {
		got = append(got, streamEv{p, tool})
	})
	stream := `{"type":"thinking","subtype":"delta","text":"x"}
{"type":"thinking","subtype":"completed","session_id":"s"}
{"type":"tool_call","subtype":"started","call_id":"1","tool_call":{},"session_id":"s"}
{"type":"tool_call","subtype":"completed","call_id":"1","tool_call":{},"session_id":"s"}
{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"s"}
`
	_, _, _, err := readCursorStreamJSON(ctx, strings.NewReader(stream), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []streamEv{
		{"thinking", ""},
		{"idle", ""},
		{"tool_call", "tool"},
		{"idle", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestReadCursorStreamJSON_ParallelToolsShowsKindThenFallback(t *testing.T) {
	var got []streamEv
	ctx := ContextWithStreamPhaseCallback(context.Background(), func(p, tool string) {
		got = append(got, streamEv{p, tool})
	})
	stream := `{"type":"tool_call","subtype":"started","call_id":"g1","tool_call":{"globToolCall":{}},"session_id":"s"}
{"type":"tool_call","subtype":"started","call_id":"r1","tool_call":{"readToolCall":{}},"session_id":"s"}
{"type":"tool_call","subtype":"completed","call_id":"r1","tool_call":{},"session_id":"s"}
{"type":"tool_call","subtype":"completed","call_id":"g1","tool_call":{},"session_id":"s"}
{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"s"}
`
	_, _, _, err := readCursorStreamJSON(ctx, strings.NewReader(stream), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []streamEv{
		{"tool_call", "glob"},
		{"tool_call", "read"},
		{"tool_call", "glob"},
		{"idle", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}
