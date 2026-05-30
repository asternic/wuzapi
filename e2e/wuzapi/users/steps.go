package users

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	"wuzapi/e2e/apiclient"
	"wuzapi/e2e/appium"
	whatsappapp "wuzapi/e2e/whatsapp/app"
	"wuzapi/e2e/whatsapp/devicename"
	"wuzapi/e2e/whatsapp/linkeddevices"
	"wuzapi/e2e/whatsapp/pairing"
	"wuzapi/e2e/whatsapp/scenario"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return newUserStateContext(ctx), nil
	})

	scenarioContext.Step(`^a WuzAPI instance named "([^"]*)"$`, func(ctx context.Context, instanceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		user, err := state.API().CreateUser(ctx, instanceName)
		if err != nil {
			return ctx, err
		}

		setCurrent(ctx, user)
		return ctx, nil
	})

	scenarioContext.Step(`^a linked WuzAPI instance named "([^"]*)"$`, func(ctx context.Context, instanceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		if err := ensureLinked(ctx, state, instanceName); err != nil {
			return ctx, err
		}

		return ctx, nil
	})

	scenarioContext.Step(`^WuzAPI requests a pairing code for "([^"]*)"$`, func(ctx context.Context, instanceName string) (context.Context, error) {
		state := scenario.FromContext(ctx)
		user, err := RequireCurrent(ctx)
		if err != nil {
			return ctx, err
		}
		if user.Name != instanceName {
			return ctx, fmt.Errorf("scenario user is %q, but the pairing code was requested for %q", user.Name, instanceName)
		}

		pairCode, err := state.API().RequestPairCode(ctx, user, state.Config().PairPhone)
		if err != nil {
			return ctx, err
		}

		pairing.SetPairCode(ctx, pairCode)
		return ctx, nil
	})
}

func ensureLinked(ctx context.Context, state *scenario.State, instanceName string) error {
	user, err := state.API().EnsureUser(ctx, instanceName)
	if err != nil {
		return err
	}
	setCurrent(ctx, user)

	if err := state.API().Connect(ctx, user); err != nil {
		return err
	}

	status, err := state.API().Status(ctx, user)
	if err == nil && status.Connected && status.LoggedIn {
		return nil
	}

	device, err := whatsappapp.Device(ctx, "WhatsApp is not open yet")
	if err != nil {
		config := state.Config()
		device, err = appiumSession(config)
		if err != nil {
			return err
		}

		whatsappapp.SetDevice(ctx, device)
		if err := whatsappapp.Open(device, config.AppPackage, config.AppActivity); err != nil {
			return err
		}
	}

	if err := linkeddevices.Open(device, state.Config().AppPackage); err != nil {
		return err
	}

	if err := linkeddevices.RemoveIfListed(device, state.Config().AppPackage, instanceName); err != nil {
		return err
	}

	if err := pairing.TapOption(device, state.Config().AppPackage, "Link a device"); err != nil {
		return err
	}

	if err := pairing.TapOption(device, state.Config().AppPackage, "Link with phone number"); err != nil {
		return err
	}

	pairCode, err := state.API().RequestPairCode(ctx, user, state.Config().PairPhone)
	if err != nil {
		return err
	}

	pairing.SetPairCode(ctx, pairCode)
	if err := pairing.TypeCode(device, state.Config().AppPackage, pairCode); err != nil {
		return err
	}

	if err := devicename.Enter(device, instanceName); err != nil {
		return err
	}

	return waitUntilLoggedIn(ctx, state, user, 90*time.Second)
}

func appiumSession(config scenario.Config) (*appium.Session, error) {
	return appium.NewSession(config.AppiumCapabilities())
}

func waitUntilLoggedIn(ctx context.Context, state *scenario.State, user *apiclient.ScenarioUser, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}

		status, err := state.API().Status(ctx, user)
		if err == nil && status.Connected && status.LoggedIn {
			return nil
		}
		if err != nil {
			lastErr = err
		}

		time.Sleep(time.Second)
	}

	if lastErr == nil {
		return fmt.Errorf("WuzAPI instance %q did not become linked in %s", user.Name, timeout)
	}
	return fmt.Errorf("WuzAPI instance %q did not become linked in %s: %w", user.Name, timeout, lastErr)
}
