package pairing

import "wuzapi/e2e/appium"

func connectDeviceSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "link_device_button")),
		appium.ByXPath(`//*[contains(@resource-id, ":id/link_device_button")]`),
		appium.ByUiAutomator(`new UiSelector().text("Link a device")`),
		appium.ByUiAutomator(`new UiSelector().text("CONECTAR DISPOSITIVO")`),
		appium.ByUiAutomator(`new UiSelector().text("Conectar dispositivo")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Link")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Conectar")`),
		appium.ByXPath(`//*[@text="Link a device" or contains(@text, "Link")]`),
		appium.ByXPath(`//*[@text="CONECTAR DISPOSITIVO" or @text="Conectar dispositivo" or contains(@text, "Conectar")]`),
	}
}

func connectWithPhoneNumberSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "bottom_banner")),
		appium.ByXPath(`//*[contains(@resource-id, ":id/bottom_banner")]`),
		appium.ByUiAutomator(`new UiSelector().text("Link with phone number")`),
		appium.ByUiAutomator(`new UiSelector().text("Link with phone number instead")`),
		appium.ByUiAutomator(`new UiSelector().text("Conectar com número de telefone")`),
		appium.ByUiAutomator(`new UiSelector().textContains("phone number")`),
		appium.ByUiAutomator(`new UiSelector().textContains("número de telefone")`),
		appium.ByXPath(`//*[@text="Link with phone number" or @text="Link with phone number instead" or contains(@text, "phone number")]`),
		appium.ByXPath(`//*[@text="Conectar com número de telefone" or contains(@text, "número de telefone")]`),
	}
}

func permissionAllowSelectors() []appium.Selector {
	return []appium.Selector{
		appium.ByID("com.android.permissioncontroller:id/permission_allow_button"),
		appium.ByID("com.android.permissioncontroller:id/permission_allow_foreground_only_button"),
		appium.ByID("com.google.android.permissioncontroller:id/permission_allow_button"),
		appium.ByID("com.google.android.permissioncontroller:id/permission_allow_foreground_only_button"),
		appium.ByID("android:id/button1"),
		appium.ByUiAutomator(`new UiSelector().text("Allow")`),
		appium.ByUiAutomator(`new UiSelector().text("Permitir")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Allow")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Permitir")`),
		appium.ByXPath(`//*[@text="Allow" or contains(@text, "Allow")]`),
		appium.ByXPath(`//*[@text="Permitir" or contains(@text, "Permitir")]`),
	}
}

func pairCodeScreenSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "enter_code_boxes")),
		appium.ByXPath(`//*[contains(@resource-id, ":id/enter_code_boxes")]`),
	}
}

func pairCodeFirstFieldSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByXPath(`//*[@resource-id="` + resourceID(appPackage, "enter_code_boxes") + `"]//android.widget.EditText[1]`),
		appium.ByXPath(`//*[contains(@resource-id, ":id/enter_code_boxes")]//android.widget.EditText[1]`),
		appium.ByUiAutomator(`new UiSelector().descriptionContains("field 1 of 8")`),
		appium.ByUiAutomator(`new UiSelector().descriptionContains("campo 1 de 8")`),
		appium.ByUiAutomator(`new UiSelector().className("android.widget.EditText")`),
		appium.ByXPath(`//*[@class="android.widget.EditText" or contains(@content-desc, "field 1 of 8")]`),
		appium.ByXPath(`//*[@class="android.widget.EditText" or contains(@content-desc, "campo 1 de 8")]`),
	}
}

func focusedPairCodeFieldSelectors() []appium.Selector {
	return []appium.Selector{
		appium.ByUiAutomator(`new UiSelector().className("android.widget.EditText").focused(true)`),
		appium.ByXPath(`//*[@class="android.widget.EditText" and @focused="true"]`),
	}
}

func resourceID(appPackage string, id string) string {
	return appPackage + ":id/" + id
}
