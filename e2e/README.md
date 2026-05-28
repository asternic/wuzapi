# WuzAPI E2E

The E2E suite runs the real WuzAPI HTTP server and drives WhatsApp on an Android device through Appium.

## Requirements

- Android Platform Tools available in `PATH` (`adb`).
- One Android device connected and authorized in `adb devices`.
- Appium running at `E2E_APPIUM_URL` or `http://127.0.0.1:4723`.
- `ANDROID_HOME` or `ANDROID_SDK_ROOT` exported before starting local Appium.
- WhatsApp installed on the Android device.
- WhatsApp already configured and logged in.

## Configuration

Copy `.env.sample` to `.env` and adjust the E2E block:

```env
E2E_APPIUM_URL=http://127.0.0.1:4723
E2E_ANDROID_UDID=
E2E_ANDROID_DEVICE_NAME=Android
E2E_WHATSAPP_PACKAGE=com.whatsapp
E2E_WHATSAPP_ACTIVITY=com.whatsapp.Main
E2E_PAIR_PHONE=5511999999999
```

Use `com.whatsapp.w4b` for the business app variant.

## Running

Start Appium from a shell with the Android SDK variables exported:

```bash
export ANDROID_HOME="$HOME/Library/Android/sdk"
export ANDROID_SDK_ROOT="$ANDROID_HOME"
appium --port 4723 --base-path /
```

Then run:

```bash
go test ./e2e
```

The test performs a preflight check before starting the scenario and fails early when Appium, ADB, the Android device, the WhatsApp package, or `E2E_PAIR_PHONE` are not ready.
