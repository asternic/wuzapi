package devicename

import "wuzapi/e2e/appium"

func deviceNameFieldSelectors() []appium.Selector {
	return []appium.Selector{
		appium.ByUiAutomator(`new UiSelector().className("android.widget.EditText").textContains("Device name")`),
		appium.ByUiAutomator(`new UiSelector().className("android.widget.EditText").textContains("Nome do dispositivo")`),
		appium.ByUiAutomator(`new UiSelector().className("android.widget.EditText")`),
		appium.ByXPath(`//*[@class="android.widget.EditText" and (contains(@text, "Device name") or contains(@content-desc, "Device name"))]`),
		appium.ByXPath(`//*[@class="android.widget.EditText" and (contains(@text, "Nome do dispositivo") or contains(@content-desc, "Nome do dispositivo"))]`),
		appium.ByXPath(`//*[@class="android.widget.EditText"]`),
	}
}

func saveDeviceNameSelectors() []appium.Selector {
	return []appium.Selector{
		appium.ByUiAutomator(`new UiSelector().text("SAVE")`),
		appium.ByUiAutomator(`new UiSelector().text("Save")`),
		appium.ByUiAutomator(`new UiSelector().text("SALVAR")`),
		appium.ByUiAutomator(`new UiSelector().text("Salvar")`),
		appium.ByXPath(`//*[@text="SAVE" or @text="Save"]`),
		appium.ByXPath(`//*[@text="SALVAR" or @text="Salvar"]`),
	}
}
