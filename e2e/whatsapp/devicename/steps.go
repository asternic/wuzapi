package devicename

import (
	"context"

	"github.com/cucumber/godog"

	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^I name the linked device "([^"]*)"$`, func(ctx context.Context, deviceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("WhatsApp must be open before entering the device name")
		if err != nil {
			return ctx, err
		}

		return ctx, Enter(device, deviceName)
	})
}
