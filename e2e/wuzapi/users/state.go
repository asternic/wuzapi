package users

import (
	"context"
	"fmt"

	"wuzapi/e2e/apiclient"
)

type stateKey struct{}

type state struct {
	user *apiclient.ScenarioUser
}

func newUserStateContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateKey{}, &state{})
}

func Current(ctx context.Context) *apiclient.ScenarioUser {
	state := stateFromContext(ctx)
	if state == nil {
		return nil
	}

	return state.user
}

func RequireCurrent(ctx context.Context) (*apiclient.ScenarioUser, error) {
	user := Current(ctx)
	if user == nil {
		return nil, fmt.Errorf("a WuzAPI instance must exist before this step")
	}

	return user, nil
}

func setCurrent(ctx context.Context, user *apiclient.ScenarioUser) {
	state := stateFromContext(ctx)
	if state != nil {
		state.user = user
	}
}

func stateFromContext(ctx context.Context) *state {
	state, _ := ctx.Value(stateKey{}).(*state)
	return state
}
