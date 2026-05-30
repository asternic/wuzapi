package linkeddevices

import (
	"fmt"
	"time"

	"wuzapi/e2e/appium"
)

func Open(device *appium.Session, appPackage string) error {
	if err := device.WaitAndTap("more options menu", overflowMenuSelectors(appPackage), 15*time.Second); err != nil {
		return err
	}

	if err := device.WaitAndTap("Linked devices option", linkedDevicesSelectors(appPackage), 15*time.Second); err != nil {
		return err
	}

	if _, err := device.WaitForElement("linked devices screen", linkedDevicesScreenSelectors(appPackage), 30*time.Second); err != nil {
		return fmt.Errorf("the linked devices list was not visible: %w", err)
	}

	return nil
}

func RemoveIfListed(device *appium.Session, appPackage string, deviceName string) error {
	for attempt := 0; attempt < 5; attempt++ {
		source, err := device.PageSource()
		if err != nil {
			return err
		}

		if !appium.ContainsAnyFold(source, deviceName) {
			return nil
		}

		if err := removeListedDevice(device, appPackage, deviceName); err != nil {
			return err
		}
	}

	return fmt.Errorf("device %q still appears in the linked devices list after cleanup", deviceName)
}

func removeListedDevice(device *appium.Session, appPackage string, deviceName string) error {
	if err := device.WaitAndTap("device "+deviceName, deviceSelectors(deviceName), 10*time.Second); err != nil {
		return err
	}

	if err := device.TapIfVisible("remove action for device "+deviceName, removeLinkedDeviceSelectors(appPackage), 3*time.Second); err != nil {
		_ = device.TapIfVisible("device actions menu for "+deviceName, overflowMenuSelectors(appPackage), 5*time.Second)

		if err := device.WaitAndTap("remove action for device "+deviceName, removeLinkedDeviceSelectors(appPackage), 15*time.Second); err != nil {
			return err
		}
	}

	_ = device.TapIfVisible("remove confirmation for device "+deviceName, confirmRemoveLinkedDeviceSelectors(appPackage), 5*time.Second)
	time.Sleep(500 * time.Millisecond)
	return nil
}

func AssertListed(device *appium.Session, appPackage string, deviceName string) error {
	source, err := device.PageSource()
	if err != nil {
		return err
	}

	if appium.ContainsAnyFold(source, "Device name", "Nome do dispositivo") {
		return fmt.Errorf("WhatsApp is still on the device name screen; use the device name step before validating")
	}

	if err := waitUntilPairCodeScreenGone(device, appPackage, 75*time.Second); err != nil {
		return fmt.Errorf("WhatsApp stayed on the pairing code screen after typing: %w", err)
	}

	source, err = device.PageSource()
	if err != nil {
		return err
	}

	if appium.ContainsAnyFold(source, deviceName) {
		return nil
	}

	if appium.ContainsAnyFold(
		source,
		"Couldn't link device",
		"Check that you entered the correct phone number",
		"request a new code",
	) {
		return fmt.Errorf("WhatsApp rejected the pairing code for %q. Check E2E_PAIR_PHONE and request a new code", deviceName)
	}

	if err := ensureVisible(device, appPackage); err != nil {
		return err
	}

	if err := device.WaitForText(45*time.Second, deviceName); err != nil {
		return fmt.Errorf("device %q did not appear in the linked devices list: %w", deviceName, err)
	}

	return nil
}

func ensureVisible(device *appium.Session, appPackage string) error {
	if _, err := device.FindFirst(linkedDevicesScreenSelectors(appPackage)); err == nil {
		return nil
	}

	return Open(device, appPackage)
}

func waitUntilPairCodeScreenGone(device *appium.Session, appPackage string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if _, err := device.FindFirst(pairCodeScreenSelectors(appPackage)); err != nil {
			return nil
		}

		lastErr = fmt.Errorf("pairing code fields are still visible")
		time.Sleep(500 * time.Millisecond)
	}

	return lastErr
}
