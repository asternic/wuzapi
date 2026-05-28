package app

import (
	"fmt"
	"time"

	"wuzapi/e2e/appium"
)

func Open(device *appium.Session, appPackage string, appActivity string) error {
	_ = device.TerminateApp(appPackage)
	time.Sleep(500 * time.Millisecond)

	if err := device.ActivateApp(appPackage, appActivity); err != nil {
		return fmt.Errorf("failed to reopen WhatsApp from scratch: %w", err)
	}

	if err := device.WaitForPackage(30*time.Second, appPackage); err != nil {
		return fmt.Errorf("WhatsApp did not render after opening; make sure the app is installed and configured: %w", err)
	}

	return nil
}
