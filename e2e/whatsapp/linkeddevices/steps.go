package linkeddevices

import (
	"context"

	"github.com/cucumber/godog"

	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^the linked devices screen is open$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("WhatsApp must be open before opening linked devices")
		if err != nil {
			return ctx, err
		}

		return ctx, Open(device, state.Config().AppPackage)
	})

	scenarioContext.Step(`^no device named "([^"]*)" is currently linked$`, func(ctx context.Context, deviceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("the linked devices list must be open before removing a device")
		if err != nil {
			return ctx, err
		}

		return ctx, RemoveIfListed(device, state.Config().AppPackage, deviceName)
	})

	scenarioContext.Step(`^"([^"]*)" appears in the list of linked devices$`, func(ctx context.Context, deviceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		device, err := state.Device("WhatsApp must be open before validating the device list")
		if err != nil {
			return ctx, err
		}

		return ctx, AssertListed(device, state.Config().AppPackage, deviceName)
	})
}
