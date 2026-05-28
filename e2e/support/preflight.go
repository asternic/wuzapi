package support

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type PreflightConfig struct {
	AppiumURLs      []string
	AndroidUDID     string
	WhatsAppPackage string
	PairPhone       string
}

type androidDevice struct {
	udid  string
	state string
}

func CheckPreflight(ctx context.Context, config PreflightConfig) error {
	if err := validatePairPhone(config.PairPhone); err != nil {
		return err
	}

	adbPath, err := exec.LookPath("adb")
	if err != nil {
		return errors.New("adb was not found in PATH; install Android Platform Tools and make adb available before running the e2e suite")
	}

	if err := ensureAndroidSDKConfigured(config.AppiumURLs, adbPath); err != nil {
		return err
	}

	deviceUDID, err := findAndroidDevice(ctx, adbPath, config.AndroidUDID)
	if err != nil {
		return err
	}

	if err := ensurePackageInstalled(ctx, adbPath, deviceUDID, config.WhatsAppPackage); err != nil {
		return err
	}

	if err := ensureAppiumIsReady(ctx, config.AppiumURLs); err != nil {
		return err
	}

	return nil
}

func validatePairPhone(pairPhone string) error {
	pairPhone = strings.TrimSpace(pairPhone)
	if pairPhone == "" {
		return errors.New("E2E_PAIR_PHONE is required; set it to the primary WhatsApp phone number without a leading +")
	}

	if strings.HasPrefix(pairPhone, "+") {
		return errors.New("E2E_PAIR_PHONE must not start with +; use only the country code and phone number digits")
	}

	for _, character := range pairPhone {
		if character < '0' || character > '9' {
			return fmt.Errorf("E2E_PAIR_PHONE must contain only digits; received %q", pairPhone)
		}
	}

	return nil
}

func ensureAndroidSDKConfigured(appiumURLs []string, adbPath string) error {
	if !usesLocalAppium(appiumURLs) {
		return nil
	}

	if os.Getenv("ANDROID_HOME") != "" || os.Getenv("ANDROID_SDK_ROOT") != "" {
		return nil
	}

	suggestion := ""
	if sdkRoot := inferSDKRoot(adbPath); sdkRoot != "" {
		suggestion = "; for this machine, try: export ANDROID_HOME=" + sdkRoot + " ANDROID_SDK_ROOT=" + sdkRoot
	}

	return errors.New("ANDROID_HOME or ANDROID_SDK_ROOT must be exported before starting local Appium" + suggestion)
}

func usesLocalAppium(appiumURLs []string) bool {
	for _, appiumURL := range appiumURLs {
		parsedURL, err := url.Parse(appiumURL)
		if err != nil {
			continue
		}

		host := parsedURL.Hostname()
		if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return true
		}
	}

	return len(appiumURLs) == 0
}

func inferSDKRoot(adbPath string) string {
	resolvedPath, err := filepath.EvalSymlinks(adbPath)
	if err != nil {
		resolvedPath = adbPath
	}

	platformToolsDir := filepath.Dir(resolvedPath)
	if filepath.Base(platformToolsDir) != "platform-tools" {
		return ""
	}

	return filepath.Dir(platformToolsDir)
}

func findAndroidDevice(ctx context.Context, adbPath string, configuredUDID string) (string, error) {
	output, err := runADB(ctx, adbPath, "devices")
	if err != nil {
		return "", fmt.Errorf("failed to list Android devices with adb: %w", err)
	}

	devices := parseADBDevices(output)
	if len(devices) == 0 {
		return "", errors.New("adb did not report any Android device; connect a device or start an emulator before running the e2e suite")
	}

	if configuredUDID != "" {
		for _, device := range devices {
			if device.udid != configuredUDID {
				continue
			}

			if device.state != "device" {
				return "", fmt.Errorf("Android device %s is %s; authorize USB debugging and wait until adb reports it as device", configuredUDID, device.state)
			}

			return configuredUDID, nil
		}

		return "", fmt.Errorf("E2E_ANDROID_UDID is %q, but adb devices did not list that device", configuredUDID)
	}

	readyDevices := make([]androidDevice, 0, len(devices))
	for _, device := range devices {
		if device.state == "device" {
			readyDevices = append(readyDevices, device)
		}
	}

	if len(readyDevices) == 1 {
		return readyDevices[0].udid, nil
	}

	if len(readyDevices) > 1 {
		return "", fmt.Errorf("adb reported multiple ready Android devices (%s); set E2E_ANDROID_UDID to choose one", describeDevices(readyDevices))
	}

	return "", fmt.Errorf("adb found Android devices, but none are ready (%s); authorize USB debugging and wait until adb reports a device state", describeDevices(devices))
}

func ensurePackageInstalled(ctx context.Context, adbPath string, udid string, appPackage string) error {
	appPackage = strings.TrimSpace(appPackage)
	if appPackage == "" {
		return errors.New("E2E_WHATSAPP_PACKAGE is required")
	}

	output, err := runADB(ctx, adbPath, "-s", udid, "shell", "pm", "path", appPackage)
	if err != nil {
		return fmt.Errorf("failed to check whether %s is installed on Android device %s: %w", appPackage, udid, err)
	}

	if !strings.Contains(output, "package:") {
		return fmt.Errorf("Android device %s does not have package %s installed; install WhatsApp or set E2E_WHATSAPP_PACKAGE", udid, appPackage)
	}

	return nil
}

func ensureAppiumIsReady(ctx context.Context, appiumURLs []string) error {
	if len(appiumURLs) == 0 {
		return errors.New("no Appium URL was configured")
	}

	client := http.Client{Timeout: 2 * time.Second}
	var errorsByURL []string

	for _, appiumURL := range appiumURLs {
		statusURL := strings.TrimRight(appiumURL, "/") + "/status"
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			errorsByURL = append(errorsByURL, fmt.Sprintf("%s: %v", statusURL, err))
			continue
		}

		response, err := client.Do(request)
		if err != nil {
			errorsByURL = append(errorsByURL, fmt.Sprintf("%s: %v", statusURL, err))
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		_ = response.Body.Close()

		if response.StatusCode >= 200 && response.StatusCode <= 299 {
			return nil
		}

		errorsByURL = append(errorsByURL, fmt.Sprintf("%s: HTTP %d %s", statusURL, response.StatusCode, strings.TrimSpace(string(body))))
	}

	return fmt.Errorf("Appium is not ready; start Appium at E2E_APPIUM_URL or 127.0.0.1:4723. Attempts: %s", strings.Join(errorsByURL, "; "))
}

func parseADBDevices(output string) []androidDevice {
	lines := strings.Split(output, "\n")
	devices := make([]androidDevice, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		devices = append(devices, androidDevice{
			udid:  fields[0],
			state: fields[1],
		})
	}

	return devices
}

func describeDevices(devices []androidDevice) string {
	descriptions := make([]string, 0, len(devices))
	for _, device := range devices {
		descriptions = append(descriptions, device.udid+"="+device.state)
	}

	return strings.Join(descriptions, ", ")
}

func runADB(ctx context.Context, adbPath string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	command := exec.CommandContext(commandCtx, adbPath, args...)
	output, err := command.CombinedOutput()
	if commandCtx.Err() != nil {
		return string(output), commandCtx.Err()
	}

	if err != nil {
		return string(output), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}
