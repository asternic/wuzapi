package appium

type Capabilities struct {
	DeviceName  string
	AppPackage  string
	AppActivity string
	UDID        string
}

func (capabilities Capabilities) legacy() map[string]interface{} {
	legacyCapabilities := map[string]interface{}{
		"platformName":      "Android",
		"automationName":    "UiAutomator2",
		"deviceName":        capabilities.DeviceName,
		"appPackage":        capabilities.AppPackage,
		"appActivity":       capabilities.AppActivity,
		"appWaitActivity":   "*",
		"autoLaunch":        false,
		"noReset":           true,
		"newCommandTimeout": 120,
	}

	if capabilities.UDID != "" {
		legacyCapabilities["udid"] = capabilities.UDID
	}

	return legacyCapabilities
}

func (capabilities Capabilities) w3c() map[string]interface{} {
	w3cCapabilities := map[string]interface{}{
		"platformName":              "Android",
		"appium:automationName":     "UiAutomator2",
		"appium:deviceName":         capabilities.DeviceName,
		"appium:appPackage":         capabilities.AppPackage,
		"appium:appActivity":        capabilities.AppActivity,
		"appium:appWaitActivity":    "*",
		"appium:autoLaunch":         false,
		"appium:noReset":            true,
		"appium:newCommandTimeout":  120,
		"appium:forceAppLaunch":     true,
		"appium:shouldTerminateApp": false,
	}

	if capabilities.UDID != "" {
		w3cCapabilities["appium:udid"] = capabilities.UDID
	}

	return w3cCapabilities
}
