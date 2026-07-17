# aero-shutter

The fastest way to pull photos off a **Nikon D5300** over Wi-Fi — straight from
the camera to your disk, no USB cable, no vendor app. aero-shutter speaks the
camera's native PTP/IP protocol over TCP and wraps it in a clean, keyboard-driven
terminal UI in the spirit of lazygit and btop.

> Screenshot coming soon.

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
│     ...                                   │      Type: NEF   Size: 24.1 MB   │
│                                           │  Captured: 2026-07-16 18:42:10   │
│                                           │    Status: not imported          │
├───────────────────────────────────────────┴──────────────────────────────────┤
│ importing DSC_0214.NEF (3/12)                                                │
│ ████████████████████░░░░░░░░░░░░░░░░  2.8 MB/s · 9 left · eta 1m12s          │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Features

- **Native PTP/IP over TCP** — talks directly to the camera on port 15740,
  implementing the ISO 15740 transport (init handshake, command/data/event
  connections, transactions).
- **Streaming, resumable downloads** — files are streamed to disk in 1 MiB
  chunks via `GetPartialObject`, written to a `.part` file and renamed on
  completion. Interrupted transfers resume from where they stopped. Memory
  stays flat no matter how large the file.
- **Instant dedupe** — every imported object is recorded in a local SQLite
  database, so repeat imports skip existing files immediately.
- **Organized library** — photos land in `~/Pictures/Nikon/YYYY/MM-DD/` based
  on capture date (configurable).
- **Thumbnails in your terminal** — embedded JPEG previews rendered inline via
  the Kitty graphics protocol (Kitty, WezTerm, Ghostty) or iTerm2 inline
  images (iTerm2, mintty, VS Code), with a bilinear-scaled ANSI half-block
  renderer as a universal fallback so previews work in every color terminal,
  including macOS Terminal.app. On Nikon bodies that support it, the larger
  vendor preview (GetLargeThumb) is fetched automatically for a sharper image.
- **Mouse support** — scroll the file list with the wheel, click rows to move
  the cursor, click again to select, double-click for the large preview, and
  click the help-bar shortcuts.
- **Camera control** — press `t` to open a control panel that shows live
  camera settings (mode, aperture, shutter speed, ISO, exposure compensation,
  white balance, capture mode, battery), lets you step writable values with
  `◀`/`▶`, and triggers the shutter remotely (`T`). After a capture the file
  list refreshes automatically so the new photo appears. Wi-Fi remote control
  support varies by Nikon body — the D5300's WU-1a-era Wi-Fi may reject some
  or all of it; everything degrades gracefully (unsupported settings are
  hidden, read-only ones are dimmed, and a clear message is shown if the body
  refuses remote capture).
- **Multiple cameras** — the scan finds every camera on your subnets; a picker
  lists saved and discovered bodies, and `c` switches cameras at any time.
- **Auto-detection** — probes the configured/default addresses and quickly
  scans your local /24 for cameras when they aren't there.
- **Watch mode** — poll the camera every 5 seconds and (optionally)
  auto-import new shots as you take them.
- **Filters & selection** — import everything, only new files, only RAW, only
  JPEG, or a hand-picked selection.
- **Cross-platform** — pure Go (no CGO), single static binary for macOS,
  Linux and Windows.

## Install

With Go 1.25+:

```sh
go install github.com/subhashraveendran/aero-shutter/cmd/aero-shutter@latest
```

Or build from source:

```sh
git clone https://github.com/subhashraveendran/aero-shutter
cd aero-shutter
make build          # binary lands in ./dist/aero-shutter
```

## Connecting to the D5300

The D5300 hosts its own Wi-Fi network:

1. On the camera: **MENU → Setup menu → Wi-Fi → Network connection → Enable**.
2. On your computer, join the Wi-Fi network the camera announces
   (`Nikon_WU2_…` by default).
3. Run `aero-shutter`. The camera assigns itself `192.168.1.1` and is detected
   automatically; if not, enter the IP manually on the connect screen.

Only one client can talk to the camera at a time — close the Nikon mobile app
if it is connected.

## Staying online while connected to the camera

The D5300 does not join your home network — it is a Wi-Fi **access point** of
its own. A Wi-Fi radio can only be associated with one network at a time, so
the moment your computer joins `Nikon_WU2_…`, your normal Wi-Fi internet
connection drops. That is a limitation of single-radio Wi-Fi hardware, not of
the camera or of aero-shutter.

The fix is a second network interface:

- **Ethernet (or a USB-C dock with an Ethernet port)** — plug into your
  router for internet and point the built-in Wi-Fi at the camera. Simplest
  and most reliable option for laptops.
- **A cheap USB Wi-Fi adapter dedicated to the camera** — keep the built-in
  Wi-Fi on your home network and let the adapter join `Nikon_WU2_…`.
- **Two adapters on a desktop** — desktops without built-in Wi-Fi can run one
  adapter on the home network and one on the camera.

No routing configuration is needed: aero-shutter probes every candidate
address and finds the camera on whichever interface sits on the camera's
subnet (`192.168.1.x` by default), regardless of which interface carries
your internet traffic.

## Usage

```sh
aero-shutter            # start the TUI
aero-shutter -version   # print the version
```

Settings (save folder, camera IP, auto-import, open-after-import) live in
`<user config dir>/aero-shutter/config.json` and are editable in-app with `s`.

The config file also holds `preview_mode` (`"auto"`, `"halfblock"`,
`"iterm2"` or `"kitty"`) and the list of saved cameras (`cameras`), which is
updated automatically on every successful connection.

> **Blocky previews in VS Code?** The integrated terminal supports inline
> images, but only when **Terminal › Integrated: Enable Images** is turned on
> in the VS Code settings. Enable it for sharp previews, or set
> `"preview_mode": "halfblock"` in `config.json` to force the text renderer.

## Keybindings

| Key      | Action                                        |
| -------- | --------------------------------------------- |
| `q`      | quit                                          |
| `r`      | refresh file list from the camera             |
| `i`      | import new files                              |
| `a`      | import all files                              |
| `space`  | toggle selection on the highlighted file      |
| `S`      | import selected files                         |
| `enter`  | load preview for the highlighted file         |
| `f`      | cycle filter: all → new → raw → jpeg          |
| `P`      | large preview overlay                         |
| `D`      | metadata detail overlay                       |
| `O`      | open the imported file with the OS viewer     |
| `s`      | settings                                      |
| `c`      | switch camera (disconnect, back to picker)    |
| `t`      | camera control panel (settings + remote shutter) |
| `T`      | take a photo (while the control panel is open) |
| `w`      | toggle watch mode (poll camera every 5s)      |
| `x`/`esc`| cancel a running import                       |
| `↑↓`/`jk`| move the cursor                               |

### Mouse

| Gesture               | Action                                                  |
| --------------------- | ------------------------------------------------------- |
| wheel up/down         | scroll the file list (3 rows per notch)                 |
| click on a row        | move the cursor to that row                             |
| click on the cursor row | toggle its selection checkbox (same as `space`)       |
| double-click on a row | open the large preview overlay (same as `P`)            |
| click in an overlay   | close the overlay                                       |
| click a help-bar label | trigger that shortcut (`q`, `r`, `i`, `a`, `f`, `s`, `c`, `t`) |

In the camera control panel the wheel scrolls the setting rows, a click
selects a row, clicking the `◀`/`▶` arrows steps the value, and clicking
"Take photo" fires the shutter.

On the connect screen the camera picker is mouse-aware too: click an entry to
connect, scroll to move the selection.

## How it works

PTP/IP runs the Picture Transfer Protocol over two TCP connections to
port 15740:

1. The client opens the **command/data connection** and sends an
   `InitCommandRequest` with a 16-byte GUID and a friendly name; the camera
   answers with an `InitCommandAck` carrying a connection number.
2. A second **event connection** is bound to the first using that connection
   number (`InitEventRequest` / `InitEventAck`).
3. All camera operations are PTP transactions on the command connection:
   `OperationRequest` → optional data phase (`StartData`/`Data`/`EndData`) →
   `OperationResponse`. aero-shutter uses `GetDeviceInfo`, `GetStorageIDs`,
   `GetObjectHandles`, `GetObjectInfo`, `GetThumb` and `GetPartialObject`.

## Architecture

```
cmd/aero-shutter/      entry point
internal/ptpip/      PTP/IP transport: framing, handshake, transactions
internal/camera/     camera abstraction + per-model capability profiles
internal/thumbnail/  thumbnail fetch + terminal inline-image rendering
internal/importer/   import engine: dedupe, resume, streaming, progress
internal/database/   SQLite record of imported objects
internal/config/     JSON configuration
internal/frontend/   Bubble Tea models, views and styles
```

Camera capabilities are described by a `Profile` (default IP, partial-object
support, thumbnail strategy, chunk size). The importer and UI only talk to the
profile-driven `Camera` type, so supporting another body is a matter of adding
a profile.

Profiles ship for the D5200, D5300, D5500, D5600, D7100, D7200, D500, D750,
D850, Z50, Z6 and Z7; anything else falls back to a conservative generic
PTP/IP profile.

## Roadmap

- Parallel downloads for cameras whose Wi-Fi can sustain them
- Simultaneous imports from multiple cameras
- EXIF-based organization rules and custom naming templates
- Optional checksum verification after transfer

## License

MIT — see [LICENSE](LICENSE).
