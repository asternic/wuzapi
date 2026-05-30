package labels

import (
	"context"

	"github.com/cucumber/godog"
)

func RegisterSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return newLabelStateContext(ctx), nil
	})

	scenarioContext.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		labelsState := labelStateFromContext(ctx)
		if labelsState == nil {
			return ctx, nil
		}
		if err := labelsState.Close(); err != nil && scenarioErr == nil {
			return ctx, err
		}

		return ctx, nil
	})

	registerWebhookSteps(scenarioContext)
	registerLifecycleSteps(scenarioContext)
	registerAssociationSteps(scenarioContext)
	registerListingSteps(scenarioContext)
}
