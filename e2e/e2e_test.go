package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"

	"wuzapi/e2e/support"
	"wuzapi/e2e/whatsapp/scenario"
	"wuzapi/e2e/whatsapp/suite"
)

func TestMain(m *testing.M) {
	if err := support.LoadEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load e2e environment: %v\n", err)
		os.Exit(1)
	}

	config := scenario.LoadConfig()
	preflight := support.PreflightConfig{
		AppiumURLs:      config.AppiumURLs,
		AndroidUDID:     config.UDID,
		WhatsAppPackage: config.AppPackage,
		PairPhone:       config.PairPhone,
	}
	if err := support.CheckPreflight(context.Background(), preflight); err != nil {
		fmt.Fprintf(os.Stderr, "e2e preflight failed: %v\n", err)
		os.Exit(1)
	}

	server, err := support.StartServer(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start e2e HTTP server: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	server.Stop()
	os.Exit(code)
}

func TestE2E(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			suite.InitializeScenario(scenarioContext)
		},
		Options: &godog.Options{
			Format:        "pretty",
			Paths:         []string{"features"},
			Strict:        true,
			StopOnFailure: true,
			TestingT:      t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("the e2e suite failed")
	}
}
