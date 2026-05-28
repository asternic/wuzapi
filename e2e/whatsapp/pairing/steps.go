package pairing

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^I tap "([^"]*)"$`, func(ctx context.Context, label string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("WhatsApp must be open before tapping an option")
		if err != nil {
			return ctx, err
		}

		return ctx, TapOption(device, state.Config().AppPackage, label)
	})

	scenarioContext.Step(`^I choose to link a new device using a phone number$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("WhatsApp must be open before choosing how to link a device")
		if err != nil {
			return ctx, err
		}

		if err := TapOption(device, state.Config().AppPackage, "Link a device"); err != nil {
			return ctx, err
		}

		return ctx, TapOption(device, state.Config().AppPackage, "Link with phone number")
	})

	scenarioContext.Step(`^WuzAPI requests a pairing code for "([^"]*)"$`, func(ctx context.Context, instanceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		if user := state.User(); user != nil && user.Name != instanceName {
			return ctx, fmt.Errorf("scenario user is %q, but the pairing code was requested for %q", user.Name, instanceName)
		}

		pairCode, err := state.API().RequestPairCode(ctx, state.User(), state.PairPhone())
		if err != nil {
			return ctx, err
		}

		state.SetPairCode(pairCode)
		return ctx, nil
	})

	scenarioContext.Step(`^I enter that pairing code in WhatsApp$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("WhatsApp must be open before entering the pairing code")
		if err != nil {
			return ctx, err
		}

		return ctx, TypeCode(device, state.Config().AppPackage, state.PairCode())
	})
}
