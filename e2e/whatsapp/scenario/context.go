package scenario

import "context"

type contextKey struct{}

func NewContext(ctx context.Context, state *State) context.Context {
	return context.WithValue(ctx, contextKey{}, state)
}

func FromContext(ctx context.Context) *State {
	state, _ := ctx.Value(contextKey{}).(*State)
	return state
}
