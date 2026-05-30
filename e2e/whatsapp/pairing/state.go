package pairing

import "context"

type stateKey struct{}

type state struct {
	pairCode string
}

func NewStateContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateKey{}, &state{})
}

func PairCode(ctx context.Context) string {
	state := stateFromContext(ctx)
	if state == nil {
		return ""
	}

	return state.pairCode
}

func SetPairCode(ctx context.Context, pairCode string) {
	state := stateFromContext(ctx)
	if state != nil {
		state.pairCode = pairCode
	}
}

func stateFromContext(ctx context.Context) *state {
	state, _ := ctx.Value(stateKey{}).(*state)
	return state
}
