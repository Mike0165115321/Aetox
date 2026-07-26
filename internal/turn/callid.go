package turn

import "context"

// The provider's tool-call id, carried on the context while that call executes.
//
// It exists for tools that cause further work: a sub-agent's tool events have to
// name the `task` call that spawned them (ToolEvent.Parent), and
// Dispatcher.ExecuteTool takes only (name, args) — deliberately, since no other
// tool has any business knowing its own id. A context value keeps that true:
// the seam is one line in the executor, and a tool that never asks is unaffected
// (ARCHITECTURE.md §44.5).

type callIDKey struct{}

// WithCallID stamps the executing tool call's id onto ctx.
func WithCallID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, callIDKey{}, id)
}

// CallID returns the id of the tool call currently executing, or "" when the
// provider sent none / the caller is not inside a tool call.
func CallID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(callIDKey{}).(string)
	return id
}
