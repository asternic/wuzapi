package linkeddevices

import "wuzapi/e2e/appium"

func overflowMenuSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "menuitem_overflow")),
		appium.ByAccessibilityID("More options"),
		appium.ByAccessibilityID("Mais opções"),
		appium.ByUiAutomator(`new UiSelector().descriptionContains("options")`),
		appium.ByUiAutomator(`new UiSelector().descriptionContains("opções")`),
		appium.ByXPath(`//*[@content-desc="More options" or contains(@content-desc, "options")]`),
		appium.ByXPath(`//*[@content-desc="Mais opções" or contains(@content-desc, "opções")]`),
	}
}

func linkedDevicesSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByXPath(`//*[@resource-id="` + resourceID(appPackage, "title") + `" and (@text="Linked devices" or @text="Dispositivos conectados")]`),
		appium.ByUiAutomator(`new UiSelector().text("Linked devices")`),
		appium.ByUiAutomator(`new UiSelector().text("Dispositivos conectados")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Linked")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Dispositivos")`),
		appium.ByXPath(`//*[@text="Linked devices" or contains(@text, "Linked")]`),
		appium.ByXPath(`//*[@text="Dispositivos conectados" or contains(@text, "Dispositivos")]`),
	}
}

func linkedDevicesScreenSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "link_device_button")),
		appium.ByID(resourceID(appPackage, "biz_linked_device_recycler_view")),
		appium.ByXPath(`//*[contains(@resource-id, ":id/link_device_button") or contains(@resource-id, ":id/biz_linked_device_recycler_view")]`),
		appium.ByUiAutomator(`new UiSelector().text("Linked devices")`),
		appium.ByUiAutomator(`new UiSelector().text("Dispositivos conectados")`),
	}
}

func deviceSelectors(deviceName string) []appium.Selector {
	return appium.TextSelectors(deviceName)
}

func removeLinkedDeviceSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "delete_device")),
		appium.ByID(resourceID(appPackage, "remove_device")),
		appium.ByUiAutomator(`new UiSelector().text("REMOVE DEVICE")`),
		appium.ByUiAutomator(`new UiSelector().text("Remove device")`),
		appium.ByUiAutomator(`new UiSelector().text("REMOVER DISPOSITIVO")`),
		appium.ByUiAutomator(`new UiSelector().text("Remover dispositivo")`),
		appium.ByUiAutomator(`new UiSelector().text("LOG OUT")`),
		appium.ByUiAutomator(`new UiSelector().text("Log out")`),
		appium.ByUiAutomator(`new UiSelector().text("SAIR")`),
		appium.ByUiAutomator(`new UiSelector().text("Sair")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Remove")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Remover")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Log out")`),
		appium.ByUiAutomator(`new UiSelector().textContains("Sair")`),
		appium.ByXPath(`//*[@text="REMOVE DEVICE" or @text="Remove device" or @text="LOG OUT" or @text="Log out" or contains(@text, "Remove") or contains(@text, "Log out")]`),
		appium.ByXPath(`//*[@text="REMOVER DISPOSITIVO" or @text="Remover dispositivo" or @text="SAIR" or @text="Sair" or contains(@text, "Remover") or contains(@text, "Sair")]`),
	}
}

func confirmRemoveLinkedDeviceSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "button1")),
		appium.ByID("android:id/button1"),
		appium.ByUiAutomator(`new UiSelector().text("REMOVE")`),
		appium.ByUiAutomator(`new UiSelector().text("Remove")`),
		appium.ByUiAutomator(`new UiSelector().text("REMOVER")`),
		appium.ByUiAutomator(`new UiSelector().text("Remover")`),
		appium.ByUiAutomator(`new UiSelector().text("LOG OUT")`),
		appium.ByUiAutomator(`new UiSelector().text("Log out")`),
		appium.ByUiAutomator(`new UiSelector().text("SAIR")`),
		appium.ByUiAutomator(`new UiSelector().text("Sair")`),
		appium.ByXPath(`//*[@text="REMOVE" or @text="Remove" or @text="LOG OUT" or @text="Log out"]`),
		appium.ByXPath(`//*[@text="REMOVER" or @text="Remover" or @text="SAIR" or @text="Sair"]`),
	}
}

func pairCodeScreenSelectors(appPackage string) []appium.Selector {
	return []appium.Selector{
		appium.ByID(resourceID(appPackage, "enter_code_boxes")),
		appium.ByXPath(`//*[contains(@resource-id, ":id/enter_code_boxes")]`),
	}
}

func resourceID(appPackage string, id string) string {
	return appPackage + ":id/" + id
}
