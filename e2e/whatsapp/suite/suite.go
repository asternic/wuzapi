package suite

import (
	"context"

	"github.com/cucumber/godog"

	whatsappapp "wuzapi/e2e/whatsapp/app"
	"wuzapi/e2e/whatsapp/devicename"
	"wuzapi/e2e/whatsapp/linkeddevices"
	"wuzapi/e2e/whatsapp/pairing"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/wuzapi/users"
)

func InitializeScenario(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return scenario.NewContext(ctx, scenario.New()), nil
	})

	scenarioContext.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		state := scenario.FromContext(ctx)
		if state == nil {
			return ctx, nil
		}

		if err := state.Close(); err != nil && scenarioErr == nil {
			return ctx, err
		}

		return ctx, nil
	})

	users.RegisterSteps(scenarioContext)
	whatsappapp.RegisterSteps(scenarioContext)
	linkeddevices.RegisterSteps(scenarioContext)
	pairing.RegisterSteps(scenarioContext)
	devicename.RegisterSteps(scenarioContext)
}
