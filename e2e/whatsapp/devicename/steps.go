package devicename

import (
	"context"

	"github.com/cucumber/godog"

	whatsappapp "wuzapi/e2e/whatsapp/app"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^I name the linked device "([^"]*)"$`, func(ctx context.Context, deviceName string) (context.Context, error) {
		device, err := whatsappapp.Device(ctx, "WhatsApp must be open before entering the device name")
		if err != nil {
			return ctx, err
		}

		return ctx, Enter(device, deviceName)
	})
}
