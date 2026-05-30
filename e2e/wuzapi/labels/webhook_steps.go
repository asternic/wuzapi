package labels

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"wuzapi/e2e/support"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/wuzapi/users"
)

func registerWebhookSteps(scenarioContext *godog.ScenarioContext) {
	scenarioContext.Step(`^the instance is subscribed to label events$`, func(ctx context.Context) (context.Context, error) {
		state := scenario.FromContext(ctx)
		user := users.Current(ctx)
		if user == nil {
			return ctx, fmt.Errorf("a WuzAPI instance must exist before configuring label events")
		}

		recorder := support.NewWebhookRecorder()
		labelStateFromContext(ctx).setWebhookRecorder(recorder)

		return ctx, state.API().SetWebhook(ctx, user, recorder.URL(), []string{
			"LabelEdit",
			"LabelAssociationChat",
			"LabelAssociationMessage",
		})
	})
}
