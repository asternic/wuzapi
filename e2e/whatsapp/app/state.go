package app

import (
	"context"
	"errors"

	"wuzapi/e2e/appium"
)

type stateKey struct{}

type state struct {
	device *appium.Session
}

func NewStateContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateKey{}, &state{})
}

func Device(ctx context.Context, message string) (*appium.Session, error) {
	state := stateFromContext(ctx)
	if state == nil || state.device == nil {
		return nil, errors.New(message)
	}

	return state.device, nil
}

func SetDevice(ctx context.Context, device *appium.Session) {
	state := stateFromContext(ctx)
	if state != nil {
		state.device = device
	}
}

func CloseState(ctx context.Context) error {
	state := stateFromContext(ctx)
	if state == nil || state.device == nil {
		return nil
	}

	return state.device.Close()
}

func stateFromContext(ctx context.Context) *state {
	state, _ := ctx.Value(stateKey{}).(*state)
	return state
}
