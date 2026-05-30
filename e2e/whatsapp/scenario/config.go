package scenario

import (
	"os"
	"strings"

	"wuzapi/e2e/appium"
)

const (
	defaultPackageName = "com.whatsapp.w4b"
	defaultActivity    = "com.whatsapp.Main"
	defaultDeviceName  = "Android"
)

type Config struct {
	AppPackage     string
	AppActivity    string
	DeviceName     string
	UDID           string
	AppiumURLs     []string
	APIBaseURL     string
	APIAdminToken  string
	PairPhone      string
	TestContactJID string
}

func LoadConfig() Config {
	return Config{
		AppPackage:     env("E2E_WHATSAPP_PACKAGE", defaultPackageName),
		AppActivity:    env("E2E_WHATSAPP_ACTIVITY", defaultActivity),
		DeviceName:     env("E2E_ANDROID_DEVICE_NAME", defaultDeviceName),
		UDID:           os.Getenv("E2E_ANDROID_UDID"),
		AppiumURLs:     appium.URLsFromEnv(),
		APIBaseURL:     strings.TrimRight(os.Getenv("E2E_WUZAPI_BASE_URL"), "/"),
		APIAdminToken:  os.Getenv("E2E_WUZAPI_ADMIN_TOKEN"),
		PairPhone:      os.Getenv("E2E_PAIR_PHONE"),
		TestContactJID: env("E2E_TEST_CONTACT_JID", os.Getenv("E2E_PAIR_PHONE")+"@s.whatsapp.net"),
	}
}

func (config Config) AppiumCapabilities() appium.Capabilities {
	return appium.Capabilities{
		DeviceName:  config.DeviceName,
		AppPackage:  config.AppPackage,
		AppActivity: config.AppActivity,
		UDID:        config.UDID,
	}
}

func env(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
