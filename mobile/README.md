# AeroShutter Mobile

The phone companion of [**aero-shutter**](https://github.com/subhashraveendran/aero-shutter).
It connects to a Nikon D5300 (and similar Wi-Fi Nikons) over **PTP/IP** (TCP port 15740),
browses photos with thumbnails, auto-imports new shots to a destination of your choice,
offers remote camera control, and has full settings — with a polished, dark-first
photographic interface.

It speaks the same protocol as the aero-shutter command-line tool, reimplemented in
TypeScript and bridged to native TCP sockets on iOS and Android.

## Screenshots

> _Placeholder — capture on device or from the browser demo (see below)._

| Connect | Gallery | Camera control | Import queue |
| ------- | ------- | -------------- | ------------ |
| _tbd_   | _tbd_   | _tbd_          | _tbd_        |

## Live demo & downloads

Try the app right now in your browser — no camera, no install:

**https://subhashraveendran.github.io/aero-shutter/**

An Android build is attached to every
[GitHub Release](https://github.com/subhashraveendran/aero-shutter/releases/latest)
as `aero-shutter-mobile-<version>-debug.apk`. This is an **unsigned debug build**
for sideloading. To install it, enable installing from unknown sources on your
device:

- **Android 8+**: open the APK, and when prompted grant your browser or file
  manager permission to install unknown apps (**Settings → Apps → [that app] →
  Install unknown apps → Allow from this source**), then continue.
- **Older Android**: **Settings → Security → Unknown sources** and enable it.

## Demo mode (runs in any browser, no camera needed)

The web build ships a **mock camera** that speaks enough PTP/IP for the entire app to
run without hardware — around 30 canvas-generated demo photos with realistic metadata.

```bash
npm install
npm run dev
```

Open the printed URL, tap **Try demo mode**, and browse, import, and control the virtual
camera. A **Demo mode** badge is shown while the mock backend is active.

Other scripts:

```bash
npm run build      # type-check + production build to dist/
npm test           # Vitest unit + end-to-end mock suite
npm run typecheck  # tsc --noEmit
```

## Building for a phone

Native builds require the platform SDKs (not needed for demo mode). The `ios/` and
`android/` projects are already generated and committed, with the local TCP plugin wired
in.

### iPhone (Xcode + CocoaPods)

```bash
npm install
npm run build
npx cap sync ios       # copies web assets and installs pods
npx cap open ios       # opens App.xcworkspace in Xcode
```

Then select a device/simulator and run. If you are starting from scratch, `npx cap add ios`
regenerates the project. `Info.plist` already sets:

- `NSLocalNetworkUsageDescription` — required for local-network TCP on iOS 14+.
- `UIFileSharingEnabled` + `LSSupportsOpeningDocumentsInPlace` — makes the
  **Files folder** import destination visible in the Files app.

CocoaPods is required (`sudo gem install cocoapods` or `brew install cocoapods`).

### Android (Android Studio)

```bash
npm install
npm run build
npx cap sync android
npx cap open android   # opens the project in Android Studio
```

Then run on a device/emulator. `npx cap add android` regenerates the project if needed.
The manifest sets `android:usesCleartextTraffic="true"` because PTP/IP is plain TCP.

### Getting the phone onto the camera Wi-Fi

1. Enable the camera's built-in Wi-Fi (D5300: setup menu → Wi-Fi).
2. On the phone, join the camera's network (e.g. `Nikon_WU2_…`).
3. Open AeroShutter — it targets `192.168.1.1` by default, or enter the IP manually.

> **Note:** while joined to the camera's Wi-Fi your phone has **no internet access**.
> That is expected; reconnect to your normal network when finished.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  React UI (screens, Zustand store, design-system CSS)     │
├──────────────────────────────────────────────────────────┤
│  CameraService  →  PtpIpClient  (src/lib/ptpip/)          │
│     PTP/IP framing, transactions, ObjectInfo,             │
│     DevicePropDesc, property formatting, chunked          │
│     GetPartialObject streaming to Filesystem              │
├──────────────────────────────────────────────────────────┤
│  @aero-shutter/tcp-socket  (local Capacitor plugin)       │
│     • Android: Kotlin java.net.Socket (background exec)   │
│     • iOS: Swift Network.framework NWConnection           │
│     • Web: MockCameraSocket  →  DEMO MODE                 │
└──────────────────────────────────────────────────────────┘
```

- **Native TCP plugin** (`plugins/tcp-socket/`) exposes `connect / write / close` plus
  `data` / `closed` / `error` listeners. All binary payloads cross the bridge as base64.
- **PTP/IP in TypeScript** (`src/lib/ptpip/`) ports the protocol from the Go CLI:
  little-endian `{len,type,payload}` framing; the init handshake (16-byte GUID +
  UTF-16LE name + version 1.0) on the command connection and `InitEventRequest` on a
  second connection bound by connection number; transactions (OperationRequest →
  StartData/Data/EndData → OperationResponse); ObjectInfo and DevicePropDesc parsing;
  and the operations `OpenSession`, `GetStorageIDs`, `GetObjectHandles`, `GetObjectInfo`,
  `GetObject`, `GetThumb`, `GetPartialObject`, `GetDevicePropDesc` (0x1014),
  `GetDevicePropValue` (0x1015), `SetDevicePropValue` (0x1016), `InitiateCapture`
  (0x100E), and Nikon's `GetLargeThumb` (0x90C4, attempted with fallback). NEFs are
  streamed in 1 MiB windows and appended to the filesystem — never held whole in memory.
- **Demo mode** — the web plugin implementation backs a `MockCameraSocket` that
  answers the same PTP/IP operations, so the full app (browse, import, control,
  capture) works in a plain browser.

### Import destinations

Chosen in Settings:

- **Phone gallery (AeroShutter album)** — app data folder (`Directory.Data`).
- **Files folder (user-visible)** — `Documents/AeroShutter`, visible in the iOS Files
  app and the Android file manager.
- **Off (browse only)** — nothing is saved.

> **Tradeoff:** rather than depend on a third-party media plugin, imports are written
> with `@capacitor/filesystem` to a dedicated, user-visible folder. This keeps the
> dependency surface small and behaves predictably across OS versions; on iOS the files
> are exposed via `UIFileSharingEnabled`, and on Android they live under
> `Documents/AeroShutter`.

Imports are **deduped** via a Dexie (IndexedDB) ledger keyed by `filename:size`
(mirroring the CLI's SQLite dedupe), and **resumable** — a partial file is detected and
downloading continues from its current offset.

## Relationship to the aero-shutter CLI

AeroShutter Mobile is a companion to the [aero-shutter](https://github.com/subhashraveendran/aero-shutter)
command-line tool. The CLI imports from Wi-Fi Nikons over PTP/IP on the desktop; this app
brings the same protocol and the same dedupe/import model to the phone, with a touch UI,
remote control, and live camera settings.

## Project layout

```
aero-shutter-mobile/
├── src/
│   ├── lib/
│   │   ├── ptpip/          PTP/IP protocol (framing, client, ObjectInfo, devprop)
│   │   ├── camera.ts       app-level camera service + streamed imports
│   │   ├── db.ts           Dexie import ledger (dedupe)
│   │   ├── settings.ts     Preferences-backed settings
│   │   └── ...
│   ├── screens/            Connect, Gallery, Detail, Import, Control, Settings
│   ├── components/         Thumbnail, TabBar, Toasts, icons
│   ├── styles/             design tokens + global + screen CSS
│   ├── store.ts            Zustand store
│   └── main.tsx / App.tsx
├── plugins/tcp-socket/     local Capacitor plugin (@aero-shutter/tcp-socket)
│   ├── src/                TS API + web demo backend (MockCameraSocket)
│   ├── android/            Kotlin implementation
│   └── ios/                Swift implementation
├── ios/  android/          generated native projects
└── capacitor.config.ts
```

## License

MIT © 2026 Subhash Raveendran. See [LICENSE](./LICENSE).
