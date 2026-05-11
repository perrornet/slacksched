package provider

import "testing"

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
