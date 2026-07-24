<div align="center">

# AeroShutter

**Pull photos off a Wi-Fi Nikon — straight to your device. No cable, no vendor app.**

[![CI](https://github.com/subhashraveendran/aero-shutter/actions/workflows/ci.yml/badge.svg)](https://github.com/subhashraveendran/aero-shutter/actions/workflows/ci.yml)
[![Release](https://github.com/subhashraveendran/aero-shutter/actions/workflows/release.yml/badge.svg)](https://github.com/subhashraveendran/aero-shutter/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-f0a92b.svg)](LICENSE)

[**Live demo**](https://subhashraveendran.github.io/aero-shutter/) · [**Download**](https://github.com/subhashraveendran/aero-shutter/releases/latest) · [**How it works**](#how-it-works)

</div>

---

AeroShutter connects to Wi-Fi-enabled **Nikon cameras** — most **D-series** DSLRs
and **Z** mirrorless — over the camera's own **PTP/IP** protocol and streams your
shots down fast, with no USB cable and no clunky manufacturer software. It ships as
two apps that share one protocol core:

- **`aero-shutter`** — a fast, **click-first** terminal app in the spirit of
  lazygit and btop. Everything is clickable; keyboard shortcuts are optional.
  Pure Go, single static binary.
- **AeroShutter Mobile** — an **iOS &amp; Android** companion with a "darkroom
  instrument" design, one-tap connect, remote capture, and live over-the-air
  updates. Built with Capacitor + React + TypeScript — and it runs fully in your
  browser in demo mode, no camera required.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ● connected  NIKON D5300          batt 78%  ·  wifi 192.168.1.1  ·  214 files│
├───────────────────────────────────────────┬──────────────────────────────────┤
│ Files · filter: new (12)                  │ Preview                          │
│ ▸ ✓ NEF DSC_0214.NEF  24.1 MB 2026-07-16  │  ╭────────────────────────────╮  │
│     JPG DSC_0213.JPG   8.4 MB 2026-07-16  │  │                            │  │
│     NEF DSC_0212.NEF  24.3 MB 2026-07-16 ✓│  │        (thumbnail)         │  │
│     MOV DSC_0211.MOV 182.0 MB 2026-07-15 ✓│  │                            │  │
│     JPG DSC_0210.JPG   7.9 MB 2026-07-15 ✓│  ╰────────────────────────────╯  │
│     NEF DSC_0209.NEF  24.0 MB 2026-07-15 ✓│      Name: DSC_0214.NEF          │
│                                           │      Type: NEF   Size: 24.1 MB   │
├───────────────────────────────────────────┴──────────────────────────────────┤
│ importing DSC_0214.NEF (3/12)                                                │
│ ████████████████████░░░░░░░░░░░░  8.9 MB/s · 9 left · eta 41s                │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Highlights

- **Native PTP/IP over TCP** — talks directly to the camera on port 15740,
  implementing the ISO 15740 transport: the init handshake, dual command and
  event connections, and full transactions. No vendor SDK.
- **Fast, resumable transfers** — files stream to disk in **4 MiB** chunks over an
  enlarged TCP receive window with `TCP_NODELAY`. Interrupted transfers resume
  from where they stopped, and memory stays flat no matter how large the file.
- **Resilient connection** — app-level keep-alive holds the link through idle
  periods, disconnects are detected in the background the instant the socket
  dies, and the client **auto-reconnects with exponential backoff** and
  transparently re-opens the PTP session.
- **Instant, event-driven import** — reacts to the camera's live `ObjectAdded`
  events so new shots land the moment you take them, and pulls the camera's
  **"Send to smart device"** queue (the photos you mark on the body).
- **Camera-recognized identity** — announces the same fixed initiator GUID and
  friendly-name that Nikon's own app uses, so the camera treats AeroShutter as a
  returning, already-paired host instead of prompting every time.
- **Terminal thumbnails** — inline JPEG previews via the Kitty and iTerm2 graphics
  protocols, with a bilinear ANSI half-block fallback so previews render in every
  color terminal (including Terminal.app). Sharper vendor previews
  (`GetLargeThumb`) are used automatically where supported.
- **Smart, deduped library** — imports organize into `~/Pictures/Nikon/YYYY/MM-DD/`
  by capture date and are recorded in SQLite, so re-imports skip existing files
  instantly. Filter by new, RAW, JPEG, or a hand-picked selection.
- **Stays online** — on mobile the camera socket binds to Wi-Fi while your phone
  keeps cellular data (no all-or-nothing Wi-Fi takeover); on desktop, any second
  network interface carries your internet.
- **Live OTA updates (mobile)** — the app updates itself over the air from GitHub
  Releases — self-hosted, zero-cost, with automatic rollback safety.
- **Cross-platform** — one static Go binary (macOS, Linux, Windows) and a
  Capacitor app for iOS &amp; Android.

## Try it in your browser

No camera, no install — the mobile app runs fully in demo mode:

**→ [subhashraveendran.github.io/aero-shutter](https://subhashraveendran.github.io/aero-shutter/)**

## Download

Prebuilt terminal binaries and the Android APK are attached to every
[GitHub Release](https://github.com/subhashraveendran/aero-shutter/releases/latest).

**Terminal app** — download the archive for your platform, extract, and run the
`aero-shutter` binary:

| Platform | Asset |
| --- | --- |
| macOS (Apple Silicon) | `aero-shutter_<version>_macos_arm64.tar.gz` |
| macOS (Intel) | `aero-shutter_<version>_macos_x86_64.tar.gz` |
| Linux (x86_64) | `aero-shutter_<version>_linux_x86_64.tar.gz` |
| Linux (arm64) | `aero-shutter_<version>_linux_arm64.tar.gz` |
| Windows (x86_64) | `aero-shutter_<version>_windows_x86_64.zip` |

**Mobile (Android)** — sideload `aero-shutter-mobile-<version>-debug.apk` (an
unsigned debug build; enable "install from unknown sources"). After the first
install, the app keeps itself current via over-the-air updates.

## Install from source

With Go 1.25+:

```sh
go install github.com/subhashraveendran/aero-shutter/cmd/aero-shutter@latest
```

Or clone and build:

```sh
git clone https://github.com/subhashraveendran/aero-shutter
cd aero-shutter
make build          # binary lands in ./dist/aero-shutter
```

## Connecting to the camera

Connecting is **zero-config** — you never type an IP address:

1. On the camera: **MENU → Setup menu → Wi-Fi → Network connection → Enable**.
2. Join the Wi-Fi network the camera announces (`Nikon_WU2_…` by default). On
   mobile, just tap **Connect to camera** and the app joins it for you.
3. AeroShutter finds the camera automatically and opens a session — no typing.

When a camera hosts its own network it is that network's gateway and DHCP server,
so AeroShutter reads the DHCP/gateway address, probes it alongside the factory
default `192.168.1.1`, any saved cameras, and a quick subnet scan — all
concurrently — then connects to the first camera that answers. Manual IP entry is
only a fallback, tucked behind a **"Trouble connecting?"** disclosure.

> Only one client can talk to the camera at a time — close Nikon's mobile app if
> it is connected.

## Staying online while connected

A Wi-Fi radio can only join one network at a time, and the camera is its own
access point with no internet. AeroShutter handles this differently per platform:

- **Mobile** binds *only the camera socket* to Wi-Fi (requesting a Wi-Fi network
  without requiring internet) while your phone's default route stays on
  cellular — so the rest of your phone keeps working. Toggle in Settings.
- **Desktop** uses whichever interface sits on the camera's subnet, so a second
  interface (Ethernet, a USB-C dock, or a dedicated USB Wi-Fi adapter) keeps you
  online with no routing configuration.

## Usage

```sh
aero-shutter            # start the TUI
aero-shutter -version   # print the version
```

Settings (save folder, camera IP, auto-import, open-after-import, keep-alive
interval, preview mode) live in `<user config dir>/aero-shutter/config.json` and
are editable in-app with `s`.

**Click-first interface.** Everything is clickable — a button toolbar, filter
chips, row checkboxes, an ⤢ Enlarge affordance, and ✕-to-close overlays, all with
hover/press feedback. Keyboard shortcuts still work as optional accelerators; the
clickable **?** button opens the full cheatsheet. A few common keys:

| Key | Action | Key | Action |
| --- | --- | --- | --- |
| `i` / `a` | import new / all | `f` | cycle filter |
| `space` / `S` | toggle / import selection | `t` / `T` | camera panel / shutter |
| `r` | refresh from camera | `w` | toggle watch mode |
| `c` | switch camera | `s` | settings |
| `P` / `D` | preview / detail overlay | `?` | cheatsheet |

## How it works

PTP/IP runs the Picture Transfer Protocol over **two TCP connections** to port 15740:

1. The client opens the **command/data connection** and sends an
   `InitCommandRequest` with a 16-byte GUID and a friendly name; the camera
   replies with an `InitCommandAck` carrying a connection number.
2. A second **event connection** is bound to the first with that number
   (`InitEventRequest` / `InitEventAck`) — required before the session opens.
3. Camera operations are PTP transactions on the command connection:
   `OperationRequest` → optional data phase (`StartData` / `Data` / `EndData`) →
   `OperationResponse`. AeroShutter uses `GetDeviceInfo`, `GetStorageIDs`,
   `GetObjectHandles`, `GetObjectInfo`, `GetThumb`, `GetPartialObject`, and the
   Nikon vendor operations for large thumbnails and the transfer queue.

## Architecture

```
cmd/aero-shutter/    entry point
internal/ptpip/      PTP/IP transport: framing, handshake, transactions, events
internal/camera/     camera abstraction + per-model capability profiles
internal/thumbnail/  thumbnail fetch + terminal inline-image rendering
internal/importer/   import engine: dedupe, resume, streaming, progress
internal/database/   SQLite record of imported objects
internal/config/     JSON configuration
internal/frontend/   Bubble Tea models, views and styles
mobile/              AeroShutter Mobile (Capacitor + React + TypeScript)
  plugins/tcp-socket   native TCP + Wi-Fi-binding plugin (Android/iOS)
  src/lib/ptpip        the PTP/IP client, mirrored in TypeScript
```

Camera capabilities are described by a `Profile` (default IP, partial-object
support, thumbnail strategy, chunk size), so supporting another body is a matter
of adding a profile. Profiles ship for the D5300, D5500, D5600, D7100,
D7200, D500, D750, D850, Z50, Z6 and Z7; anything else falls back to a
conservative generic PTP/IP profile.

## Roadmap

- EXIF-based organization rules and custom naming templates
- Optional checksum verification after transfer
- Live View streaming and expanded remote-capture controls
- Signed release builds for seamless in-place app updates

## License

MIT — see [LICENSE](LICENSE).
