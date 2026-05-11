package provider

import "context"

type ctxKeyStreamPhase struct{}

// ContextWithStreamPhaseCallback attaches a handler for Cursor CLI stream-json phases.
// phase is "thinking" | "tool_call" | "idle"; tool is a short tool id when phase=="tool_call" (e.g. glob, read).
func ContextWithStreamPhaseCallback(ctx context.Context, fn func(phase, tool string)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyStreamPhase{}, fn)
}

func streamPhaseCallback(ctx context.Context) func(string, string) {
	f, _ := ctx.Value(ctxKeyStreamPhase{}).(func(string, string))
	return f
}
