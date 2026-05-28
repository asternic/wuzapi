package pairing

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"wuzapi/e2e/appium"
)

func TapOption(device *appium.Session, appPackage string, label string) error {
	switch label {
	case "Link a device":
		return tapConnectDevice(device, appPackage)
	case "Link with phone number":
		return tapConnectWithPhoneNumber(device, appPackage)
	default:
		return device.WaitAndTap("option "+label, appium.TextSelectors(label), 15*time.Second)
	}
}

func TypeCode(device *appium.Session, appPackage string, rawPairCode string) error {
	pairCode, err := normalizePairCode(rawPairCode)
	if err != nil {
		return err
	}

	if _, err := device.WaitForElement("pairing code entry screen", pairCodeScreenSelectors(appPackage), 30*time.Second); err != nil {
		return fmt.Errorf("the pairing code entry screen is not visible: %w", err)
	}

	firstField, err := device.WaitForElement("first pairing code field", pairCodeFirstFieldSelectors(appPackage), 15*time.Second)
	if err != nil {
		return err
	}

	if err := firstField.Click(); err != nil {
		return fmt.Errorf("failed to focus the first pairing code field: %w", err)
	}

	time.Sleep(300 * time.Millisecond)
	for _, character := range pairCode {
		targetField := firstField
		if focusedField, err := device.FindFirst(focusedPairCodeFieldSelectors()); err == nil {
			targetField = focusedField
		}

		if err := targetField.SendKeys(string(character)); err != nil {
			return fmt.Errorf("failed to type pairing code character %q: %w", character, err)
		}

		time.Sleep(250 * time.Millisecond)
	}

	return nil
}

func tapConnectDevice(device *appium.Session, appPackage string) error {
	if err := device.WaitAndTap("Link a device button", connectDeviceSelectors(appPackage), 15*time.Second); err != nil {
		return err
	}

	_ = device.TapIfVisible("camera permission", permissionAllowSelectors(), 5*time.Second)
	return nil
}

func tapConnectWithPhoneNumber(device *appium.Session, appPackage string) error {
	if err := device.WaitAndTap("Link with phone number option", connectWithPhoneNumberSelectors(appPackage), 30*time.Second); err != nil {
		return err
	}

	if _, err := device.WaitForElement("pairing code entry screen", pairCodeScreenSelectors(appPackage), 30*time.Second); err != nil {
		return fmt.Errorf("tapped Link with phone number, but the code screen was not visible: %w", err)
	}

	return nil
}

func normalizePairCode(rawPairCode string) (string, error) {
	var builder strings.Builder

	for _, character := range strings.ToUpper(rawPairCode) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
		}
	}

	pairCode := builder.String()
	if len(pairCode) != 8 {
		return "", fmt.Errorf("the pairing code returned by the API should have 8 characters; received %q", rawPairCode)
	}

	return pairCode, nil
}
