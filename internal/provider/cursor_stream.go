package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxCursorStreamLogBytes = 256 * 1024

// parseCursorResultLine returns the final assistant text and session_id from a stream-json line
// when type is "result". Other lines are ignored (hit=false).
func parseCursorResultLine(line []byte) (text, sessionID string, hit bool, err error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return "", "", false, nil
	}
	var ev struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		Result    string `json:"result"`
		SessionID string `json:"session_id"`
		IsError   bool   `json:"is_error"`
	}
	if err := json.Unmarshal(line, &ev); err != nil {
		return "", "", false, nil
	}
	if ev.Type != "result" {
		return "", "", false, nil
	}
	if ev.IsError {
		msg := strings.TrimSpace(ev.Result)
		if msg == "" {
			msg = "unknown error"
		}
		return "", ev.SessionID, true, fmt.Errorf("cursor result error: %s", msg)
	}
	if ev.Subtype != "success" {
		return "", "", false, nil
	}
	return ev.Result, ev.SessionID, true, nil
}

// streamPhaseTracker maps Cursor stream-json events to Slack Assistant phases.
// It tracks overlapping tool_call runs by call_id so status shows the actual tool name and
// updates correctly when parallel tools finish.
type streamPhaseTracker struct {
	active       []activeTool
	thinkingOpen bool
}

type activeTool struct {
	id   string
	kind string // short name, e.g. glob, read (from JSON keys like globToolCall)
}

// onStreamLine updates counters and calls emit(phase, toolShort); toolShort is empty unless phase=="tool_call".
func (s *streamPhaseTracker) onStreamLine(line []byte, emit func(phase, toolShort string)) {
	if emit == nil {
		return
	}
	var head struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
	}
	if json.Unmarshal(line, &head) != nil {
		return
	}
	switch head.Type {
	case "tool_call":
		switch head.Subtype {
		case "started":
			id := extractCursorCallID(line)
			kind := parseCursorToolCallKind(line)
			if kind == "" {
				kind = "tool"
			}
			s.active = append(s.active, activeTool{id: id, kind: kind})
			emit("tool_call", kind)
		case "completed":
			id := extractCursorCallID(line)
			s.removeActiveTool(id)
			if len(s.active) > 0 {
				emit("tool_call", s.active[len(s.active)-1].kind)
			} else if !s.thinkingOpen {
				emit("idle", "")
			}
		}
	case "thinking":
		switch head.Subtype {
		case "delta":
			if !s.thinkingOpen {
				s.thinkingOpen = true
				emit("thinking", "")
			}
		case "completed":
			s.thinkingOpen = false
			if len(s.active) == 0 {
				emit("idle", "")
			}
		}
	}
}

func (s *streamPhaseTracker) removeActiveTool(callID string) {
	if callID == "" {
		return
	}
	for i, t := range s.active {
		if t.id == callID {
			s.active = append(s.active[:i], s.active[i+1:]...)
			return
		}
	}
}

func extractCursorCallID(line []byte) string {
	var v struct {
		CallID string `json:"call_id"`
	}
	if json.Unmarshal(line, &v) != nil {
		return ""
	}
	return strings.TrimSpace(v.CallID)
}

// parseCursorToolCallKind reads tool_call.globToolCall | readToolCall | … and returns a short label.
func parseCursorToolCallKind(line []byte) string {
	var v struct {
		ToolCall json.RawMessage `json:"tool_call"`
	}
	if json.Unmarshal(line, &v) != nil || len(v.ToolCall) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(v.ToolCall, &m) != nil || len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return shortenCursorToolKey(keys[0])
}

func shortenCursorToolKey(k string) string {
	k = strings.TrimSpace(k)
	k = strings.TrimSuffix(k, "ToolCall")
	return k
}

func readCursorStreamJSON(ctx context.Context, stdout io.Reader, log *slog.Logger, trace bool) (finalText, sessionID string, assistantBuf string, err error) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var sawResult bool
	var asst strings.Builder
	var lastAssistant string // last complete assistant text line (see comment below)
	var phase streamPhaseTracker
	phaseCb := streamPhaseCallback(ctx)
	for sc.Scan() {
		rawLine := sc.Bytes()
		lineStr := normalizeCursorStreamLine(string(rawLine))
		line := []byte(lineStr)
		phase.onStreamLine(line, phaseCb)
		if trace && log != nil {
			s := lineStr
			if len(s) > maxCursorStreamLogBytes {
				s = truncateUTF8Cursor(s, maxCursorStreamLogBytes) + "…[truncated]"
			}
			log.Info("cursor_stream", "raw", s)
		}
		if p := parseCursorAssistantLine(line); p != "" {
			asst.WriteString(p)
			// Cursor CLI often emits a first assistant bubble ("Checking…") before tools,
			// then the real reply in a later assistant bubble. The terminal result.result
			// concatenates both; keep only the last assistant segment for Slack-facing text.
			lastAssistant = p
		}
		txt, sid, hit, perr := parseCursorResultLine(line)
		if perr != nil {
			return "", "", asst.String(), perr
		}
		if hit {
			sawResult = true
			finalText = txt
			if sid != "" {
				sessionID = sid
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", asst.String(), err
	}
	if !sawResult {
		return "", "", asst.String(), fmt.Errorf("no terminal result event in cursor stream-json output")
	}
	if strings.TrimSpace(lastAssistant) != "" {
		finalText = lastAssistant
	}
	return finalText, sessionID, asst.String(), nil
}

// parseCursorAssistantLine returns concatenated assistant text deltas (multica-style content blocks).
func parseCursorAssistantLine(line []byte) string {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return ""
	}
	var ev struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(line, &ev) != nil || ev.Type != "assistant" {
		return ""
	}
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(ev.Message, &msg) != nil {
		return ""
	}
	var b strings.Builder
	for _, c := range msg.Content {
		switch c.Type {
		case "text", "output_text":
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

func truncateUTF8Cursor(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
