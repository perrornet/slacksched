package provider

import "testing"

func TestCodexAgentMessageFinalAnswerOnly(t *testing.T) {
	c := &codexAppConn{}
	c.recordCodexAgentMessage("Checking…", "")
	c.recordCodexAgentMessage("我是机器人助手。", "final_answer")
	got := c.codexSlackFacingText()
	if got != "我是机器人助手。" {
		t.Fatalf("got %q want final_answer text only", got)
	}
}

func TestCodexAgentMessageFallbackWithoutFinal(t *testing.T) {
	c := &codexAppConn{}
	c.recordCodexAgentMessage("Only draft phases.", "")
	got := c.codexSlackFacingText()
	if got != "Only draft phases." {
		t.Fatalf("got %q want fallback when no final_answer", got)
	}
}

func TestCodexAgentMessageFinalEmptyIgnoresFallback(t *testing.T) {
	c := &codexAppConn{}
	c.recordCodexAgentMessage("draft", "")
	c.recordCodexAgentMessage("", "final_answer")
	got := c.codexSlackFacingText()
	want := "(no assistant text captured)"
	if got != want {
		t.Fatalf("got %q want %q (final_answer empty must not use draft)", got, want)
	}
}

func TestCodexItemPhase(t *testing.T) {
	p, d, ok := codexItemPhase("mcpToolCall", map[string]any{"tool": "my_mcp/tool"})
	if !ok || p != "tool_call" || d != "my_mcp_tool" {
		t.Fatalf("mcpToolCall: got %q %q ok=%v", p, d, ok)
	}
	p, d, ok = codexItemPhase("reasoning", map[string]any{})
	if !ok || p != "thinking" || d != "" {
		t.Fatalf("reasoning: got %q %q ok=%v", p, d, ok)
	}
	p, d, ok = codexItemPhase("commandExecution", map[string]any{})
	if !ok || p != "tool_call" || d != "shell" {
		t.Fatalf("commandExecution: got %q %q ok=%v", p, d, ok)
	}
}

func TestItemStringID(t *testing.T) {
	if itemStringID(float64(12)) != "12" {
		t.Fatal(itemStringID(float64(12)))
	}
	if itemStringID("x") != "x" {
		t.Fatal()
	}
}
