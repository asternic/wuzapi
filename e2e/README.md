# WuzAPI E2E

The E2E suite runs the real WuzAPI HTTP server and drives WhatsApp Business on an Android device through Appium.

## Requirements

- Android Platform Tools available in `PATH` (`adb`).
- One Android device connected and authorized in `adb devices`.
- Appium available in `PATH`; the preflight starts a local Appium server when `E2E_APPIUM_URL` points to localhost and no server is already running.
- WhatsApp Business installed on the Android device.
- WhatsApp Business already configured and logged in.

## Configuration

Copy `.env.sample` to `.env` and adjust the E2E block:

```env
E2E_APPIUM_URL=http://127.0.0.1:4723
E2E_ANDROID_UDID=
E2E_ANDROID_DEVICE_NAME=Android
E2E_WHATSAPP_PACKAGE=com.whatsapp.w4b
E2E_WHATSAPP_ACTIVITY=com.whatsapp.Main
E2E_PAIR_PHONE=5511999999999
# Optional for local Appium. The preflight infers these from adb when empty.
ANDROID_HOME=
ANDROID_SDK_ROOT=
```

Use `com.whatsapp` only when intentionally running the suite against the consumer app variant.

## Running

Run the suite directly:

```bash
go test ./e2e
```

The test performs a preflight check before starting the scenario. For local Appium it infers `ANDROID_HOME` and `ANDROID_SDK_ROOT` from the resolved `adb` path when they are not already set, starts Appium on the configured local URL if needed, and stops that Appium process after the suite finishes.
