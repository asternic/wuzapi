package appium

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nichotined/appium-go-client/webdriver"
)

const (
	defaultAppiumURL = "http://127.0.0.1:4723"
	defaultAppiumHub = "http://127.0.0.1:4723/wd/hub"
)

type Session struct {
	driver *webdriver.Driver
}

func NewSession(capabilities Capabilities) (*Session, error) {
	legacyCapabilities := capabilities.legacy()
	var lastErr error

	for _, appiumURL := range URLsFromEnv() {
		driver := webdriver.Create(appiumURL, legacyCapabilities)
		response, err := request(driver, "POST", "/session", map[string]interface{}{
			"capabilities": map[string]interface{}{
				"alwaysMatch": capabilities.w3c(),
				"firstMatch":  []map[string]interface{}{{}},
			},
			"desiredCapabilities": legacyCapabilities,
		})
		if err != nil {
			if response != nil && response.statusCode != 404 {
				return nil, fmt.Errorf("failed to start an Appium session at %s: %w", appiumURL, err)
			}

			lastErr = fmt.Errorf("%s: %w", appiumURL, err)
			continue
		}

		sessionID, err := parseSessionID(response.body)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", appiumURL, err)
			continue
		}

		driver.SessionID = sessionID
		return &Session{driver: driver}, nil
	}

	if lastErr == nil {
		lastErr = errors.New("no Appium URL was configured")
	}

	return nil, fmt.Errorf("failed to start an Appium session: %w", lastErr)
}

func (session *Session) Close() error {
	if session == nil || session.driver == nil || session.driver.SessionID == "" {
		return nil
	}

	_, err := session.request("DELETE", "/session/"+session.driver.SessionID, nil)
	session.driver = nil

	if err != nil && strings.Contains(err.Error(), "404") {
		return nil
	}

	return err
}

func (session *Session) TerminateApp(appPackage string) error {
	_, err := session.request("POST", session.sessionPath("/appium/device/terminate_app"), map[string]interface{}{
		"appId": appPackage,
	})

	return err
}

func (session *Session) ActivateApp(appPackage string, appActivity string) error {
	if _, err := session.request("POST", session.sessionPath("/appium/device/start_activity"), map[string]interface{}{
		"appPackage":      appPackage,
		"appActivity":     appActivity,
		"appWaitActivity": "*",
	}); err == nil {
		return nil
	}

	_, err := session.request("POST", session.sessionPath("/appium/device/activate_app"), map[string]interface{}{
		"appId": appPackage,
	})

	return err
}

func URLsFromEnv() []string {
	if appiumURL := os.Getenv("E2E_APPIUM_URL"); appiumURL != "" {
		return []string{strings.TrimRight(appiumURL, "/")}
	}

	return []string{defaultAppiumURL, defaultAppiumHub}
}

func (session *Session) sessionPath(path string) string {
	return "/session/" + session.driver.SessionID + path
}
