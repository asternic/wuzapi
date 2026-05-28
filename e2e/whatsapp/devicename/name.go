package devicename

import (
	"fmt"
	"time"

	"wuzapi/e2e/appium"
)

func Enter(device *appium.Session, deviceName string) error {
	if err := device.WaitForText(45*time.Second, "Device name", "Nome do dispositivo"); err != nil {
		return fmt.Errorf("the device name screen was not visible: %w", err)
	}

	field, err := device.WaitForElement("device name field", deviceNameFieldSelectors(), 15*time.Second)
	if err != nil {
		return err
	}

	if err := field.Click(); err != nil {
		return fmt.Errorf("failed to focus the device name field: %w", err)
	}

	if err := field.SendKeys(deviceName); err != nil {
		return fmt.Errorf("failed to type the device name: %w", err)
	}

	if err := device.WaitAndTap("Save device name button", saveDeviceNameSelectors(), 15*time.Second); err != nil {
		return err
	}

	return nil
}
