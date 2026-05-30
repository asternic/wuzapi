package app

import (
	"context"

	"github.com/cucumber/godog"

	"wuzapi/e2e/appium"
	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return NewStateContext(ctx), nil
	})

	scenarioContext.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		if err := CloseState(ctx); err != nil && scenarioErr == nil {
			return ctx, err
		}

		return ctx, nil
	})

	scenarioContext.Step(`^WhatsApp is open on the phone$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		config := state.Config()

		device, err := appium.NewSession(config.AppiumCapabilities())
		if err != nil {
			return ctx, err
		}

		SetDevice(ctx, device)
		return ctx, Open(device, config.AppPackage, config.AppActivity)
	})
}
