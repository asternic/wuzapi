package pairing

import (
	"context"

	"github.com/cucumber/godog"

	whatsappapp "wuzapi/e2e/whatsapp/app"
	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return NewStateContext(ctx), nil
	})

	scenarioContext.Step(`^I tap "([^"]*)"$`, func(ctx context.Context, label string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := whatsappapp.Device(ctx, "WhatsApp must be open before tapping an option")
		if err != nil {
			return ctx, err
		}

		return ctx, TapOption(device, state.Config().AppPackage, label)
	})

	scenarioContext.Step(`^I choose to link a new device using a phone number$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := whatsappapp.Device(ctx, "WhatsApp must be open before choosing how to link a device")
		if err != nil {
			return ctx, err
		}

		if err := TapOption(device, state.Config().AppPackage, "Link a device"); err != nil {
			return ctx, err
		}

		return ctx, TapOption(device, state.Config().AppPackage, "Link with phone number")
	})

	scenarioContext.Step(`^I enter that pairing code in WhatsApp$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := whatsappapp.Device(ctx, "WhatsApp must be open before entering the pairing code")
		if err != nil {
			return ctx, err
		}

		return ctx, TypeCode(device, state.Config().AppPackage, PairCode(ctx))
	})
}
