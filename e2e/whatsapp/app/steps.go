package app

import (
	"context"

	"github.com/cucumber/godog"

	"wuzapi/e2e/appium"
	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^WhatsApp is open on the phone$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		config := state.Config()

		device, err := appium.NewSession(config.AppiumCapabilities())
		if err != nil {
			return ctx, err
		}

		state.SetDevice(device)
		return ctx, Open(device, config.AppPackage, config.AppActivity)
	})
}
