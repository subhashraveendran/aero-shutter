I'll write the audit report directly. The data is comprehensive, so I'll organize it into the five required sections.

# aero-shutter vs Nikon WMU — Feature Parity Audit Report

## 1. Executive Summary

**Overall closeness.** aero-shutter's two clients (Go CLI + Capacitor mobile) reproduce the **core PTP/IP transport and photo-ingestion pipeline** of Nikon's Wireless Mobile Utility with high fidelity, but deliberately omit two entire WMU pillars: **Live View streaming** and **camera-side network/NCC configuration**. Against WMU's full surface, aero-shutter implements roughly the full "connect → enumerate → download" path plus basic remote capture, and skips the interactive shooting (liveview/AF/zoom) and camera-settings-provisioning features.

**Where parity is strongest (the biggest wins already in place):**
- **PTP/IP handshake, dual-socket (cmd+evt), session lifecycle, and transaction serialization** are complete and correct on both clients (`internal/ptpip/client.go:70-119`, `mobile/src/lib/ptpip/client.ts:114-183`). Both serialize transactions (Go mutex, mobile promise `txnQueue`), which WMU also does.
- **Download engine**: `GetObject`, `GetPartialObject` (4 MiB chunks), streamed writes, and **resumable transfers from partial-file offset** are present on both — a robustness feature on par with or better than WMU.
- **Deduplication**: both persist an imported-file ledger (`internal/database/database.go`, `mobile/src/lib/db.ts`) keyed on filename+size, preventing re-downloads.
- **Thumbnails**: both try `NikonGetLargeThumb (0x90C4)` first and fall back to `GetThumb (0x100A)`.

**Headline gaps:**
1. **Event-driven behavior is entirely absent.** Both clients open the event channel and then *silently drain and discard* every event (`client.go:170-191`, `client.ts:192-210`). No `ObjectAdded`, `CaptureComplete`, `StoreRemoved`, or `DevicePropChanged` handling. Auto-import is 5 s polling, not event-driven.
2. **Connection resilience is inconsistent and incomplete.** No auto-reconnect on either client; disconnect is only detected on the *next* operation; the CLI has no application-level keep-alive at all (mobile pings every 9 s via `GetStorageIDs`).
3. **Live View is missing wholesale** (no `StartLiveview`/`GetLiveviewImg`/AF/zoom) — but this is by design for an import-focused app.
4. **Camera NCC/Wi-Fi provisioning is missing wholesale** (`SetNccSSID`, auth, channel, WPS…) — also by design.
5. **No EXIF parsing/orientation/GPS geotagging** — files are passed through as-is.

**CLI vs mobile divergence** is itself a consistency risk: mobile has Wi-Fi binding, discovery/auto-connect, keep-alive, and notifications that the CLI lacks; the CLI has full `GetDeviceInfo`/`StorageInfo`/`BatteryLevel` methods and TCP socket tuning that mobile lacks. Neither is a strict superset of the other.

---

## 2. Full Feature Comparison Matrix

Legend: ✅ present · 🟡 partial · ❌ missing · — n/a

### Connection & Session Lifecycle

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| OpenPTPIP Handshake | ✅ | ✅ | ✅ | `client.go:70-100`; `client.ts:114-182` |
| GUID (Initiator) | ✅ | ✅ | ✅ | Identical hardcoded WMU GUID |
| User Agent / Friendly Name | ✅ | ✅ | ✅ | `aero-shutter/<ver>` |
| IP Address Resolution | ✅ | ✅ | ✅ | CLI parses routing tables; mobile discovery |
| Socket Connect (10 s timeout) | ✅ | 🟡 | 🟡 | CLI 5s, mobile 8s; WMU 10s |
| OpenSession | ✅ | ✅ | ✅ | SessionAlreadyOpen treated as success |
| CloseSession | ✅ | ✅ | ✅ | |
| ClosePTPIP | ✅ | ✅ | ✅ | |
| OpenState state machine | ✅ | — | — | Both use binary connected flag |
| SessionState state machine | ✅ | — | 🟡 | Mobile tracks `sessionOpen` |
| GetEvent loop | ✅ | ✅ | 🟡 | CLI buffers via `Events()`; mobile drains |
| ThreadEvent monitor | ✅ | — | — | goroutine / listener instead |
| ProbeRequest keep-alive | ✅ | 🟡 | 🟡 | Both respond; neither sends proactively |
| ThreadProbeRequest monitor | ✅ | — | 🟡 | Mobile 9 s timer (`store.ts:704-726`) |
| Disconnect Detection | ✅ | 🟡 | 🟡 | Only on next op; no background monitor |
| InitCommand/InitEvent exchange | ✅ | ✅ | ✅ | |
| Socket Pair (cmd+evt) | ✅ | ✅ | ✅ | |
| PTP Result Error Codes | ✅ | ✅ | 🟡 | Mobile missing several codes |
| Response Code Mapping | ✅ | ✅ | 🟡 | Mobile shows hex only |
| Event Codes enum | ✅ | 🟡 | 🟡 | Neither has EventCode enum |
| NccWifiKeepAliveTime setting | ✅ | — | 🟡 | Mobile hardcodes 9 s |
| TCP Window Clamp (Cat D) | ✅ | — | — | Not implemented |
| Wi-Fi Network Change Detection | ✅ | — | ✅ | `mobile/src/lib/wifi.ts` |
| Wi-Fi State Listeners | ✅ | — | ✅ | Android NetworkCallback |
| PTP lifecycle callbacks | ✅ | — | — | async patterns instead |
| Device Info Retrieval | ✅ | ✅ | ✅ | Neither has 5-retry |
| IP Retrieval (DHCP) | ✅ | ✅ | 🟡 | |
| Init Timeout (liveview wait) | ✅ | — | — | Immediate OpenSession |
| Reconnection Logic | ✅ | 🟡 | 🟡 | No true auto-reconnect |
| Error Propagation (event loss) | ✅ | — | 🟡 | CLI exits loop silently |
| Socket Factory (Wi-Fi bound) | ✅ | — | ✅ | `TcpSocketPlugin.kt:258-300` |
| Liveview state codes | ✅ | — | — | — |
| Remote Mode state | ✅ | — | 🟡 | mobile `initiateCapture` |
| NccSettingsState | ✅ | — | — | — |

### Wi-Fi Management

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| SSID Scanning | ✅ | 🟡 | ✅ | CLI is IP-based only |
| Wi-Fi Network Connection | ✅ | — | ✅ | `joinWifi()` Android 10+ |
| Saved Network Management | ✅ | — | 🟡 | Only last IP kept |
| DHCP Server Addr Retrieval | ✅ | ❌ | ✅ | CLI has infra, not wired |
| Wi-Fi Conn State Monitoring | ✅ | ❌ | 🟡 | Mobile polls on demand |
| Wi-Fi State Notifications | ✅ | ❌ | 🟡 | |
| Link Speed Detection | ✅ | ❌ | ❌ | |
| Wi-Fi Enable/Disable | ✅ | — | ❌ | |
| Connected SSID Retrieval | ✅ | ❌ | ✅ | `currentSsid()` |
| Net Change After Launch | ✅ | ❌ | ❌ | |
| Wi-Fi Lock Acquisition | ✅ | ❌ | 🟡 | TCP keepalive instead |
| Saved Network Pref Tracking | ✅ | 🟡 | 🟡 | IP only |
| Wi-Fi Config Auto-Connection | ✅ | ❌ | 🟡 | `autoConnect()` probes IPs |
| WPA2 Auth Config | ✅ | — | ✅ | `setWpa2Passphrase` |
| Open Network Support | ✅ | — | ✅ | |
| NFC Wi-Fi Tag Parsing | ✅ | ❌ | ❌ | |
| SSID Settings (PTP/NCC) | ✅ | ❌ | ❌ | Out of scope |
| Wi-Fi Auth Mode (PTP/NCC) | ✅ | ❌ | ❌ | Out of scope |
| WPA2 Passphrase (PTP/NCC) | ✅ | ❌ | ❌ | Out of scope |
| Wi-Fi Channel (PTP/NCC) | ✅ | ❌ | ❌ | Out of scope |
| WPS Mode/PIN/Timeout (PTP/NCC) | ✅ | ❌ | ❌ | Out of scope |
| Camera Wi-Fi Idle Timeout (NCC) | ✅ | ❌ | ❌ | Keep-alive compensates |
| Wi-Fi Keep-Alive (NCC) | ✅ | ❌ | ❌ | TCP+app pings instead |
| Camera IP/Mask/DHCP (NCC) | ✅ | ❌ | ❌ | Out of scope |
| Get Camera NCC Settings | ✅ | ❌ | ❌ | Out of scope |
| PTP/IP Conn Establishment | ✅ | ✅ | ✅ | |
| PTP/IP Conn Termination | ✅ | ✅ | ✅ | |
| Wi-Fi Probe Keep-Alive | ✅ | 🟡 | ✅ | |
| Wi-Fi Settings Persistence | ✅ | 🟡 | ✅ | |
| Wi-Fi Config Validation | ✅ | ❌ | ❌ | passphrase not validated |
| Wi-Fi Config Priority/History | ✅ | ❌ | ❌ | |
| Capability-Based Auth | ✅ | — | ✅ | no capability parse |
| Default Camera SSID Auto-Detect | ✅ | — | ✅ | `Nikon_WU2_` prefix |
| Network Auto-Recovery | ✅ | ❌ | 🟡 | no retry after error |
| Last Connected Net Restoration | ✅ | 🟡 | 🟡 | IP only |
| Wi-Fi Service Enable/Disable | — | — | ❌ | |
| NFC Wi-Fi Auto-Connect | ✅ | — | ❌ | |
| Wi-Fi Settings UI | ✅ | — | ❌ | Out of scope |

### Events & Auto-Transfer

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| GetEvent Polling Loop | ✅ | ❌ | ❌ | events drained/ignored |
| ObjectAdded (0x4002) | ✅ | ❌ | ❌ | |
| CaptureComplete (0x400d) | ✅ | ❌ | ❌ | |
| CaptureCompleteRecInSdram (0xc0e2) | ✅ | — | ❌ | |
| ObjectAddedInSdram (0xc0a1) | ✅ | — | ❌ | |
| DevicePropChanged (0x400e) | ✅ | ❌ | ❌ | |
| StoreRemoved (0x4009) | ✅ | ❌ | ❌ | |
| ObjectInfoChanged (0x400b) | ✅ | ❌ | ❌ | |
| DeviceInfoChanged (0x400c) | ✅ | ❌ | ❌ | |
| StoreFull (0x4012) | ✅ | ❌ | ❌ | |
| RequestObjectTransfer (0x4011) | ✅ | ❌ | ❌ | |
| ObjectRemoved (0x4007) | ✅ | ❌ | ❌ | |
| StoreAdded (0x4008) | ✅ | ❌ | ❌ | |
| CancelTransaction (0x4001) | ✅ | ❌ | ❌ | |
| Folder Handle Filtering | ✅ | ✅ | ✅ | `camera.go:165` |
| RAW Format Filtering | ✅ | ✅ | 🟡 | no compression-mode check |
| Auto-Transfer on ObjectAdded | ✅ | ❌ | 🟡 | mobile polls 5 s |
| Auto-Transfer Format Check | ✅ | 🟡 | 🟡 | implicit via enum |
| Storage Full Check pre-transfer | ✅ | ❌ | ❌ | |
| Sequential Download Queue | ✅ | ✅ | ✅ | |
| GetEventCommand (JNI) | — | — | — | n/a (no JNI) |
| GetEvent (JNI) | — | — | — | n/a |
| GetObject (JNI) | ✅ | ✅ | ✅ | |
| GetPartialObject (JNI) | ✅ | ✅ | ✅ | |
| GetTransferList | ✅ | ❌ | ❌ | |
| SetTransferListLock | ✅ | ❌ | ❌ | |
| GetCompressionSetting | ✅ | ❌ | ❌ | |
| GetNumObjects | ✅ | ❌ | ❌ | |
| GetObjectHandles | ✅ | ✅ | ✅ | |
| GetObjectInfo | ✅ | ✅ | ✅ | |
| TransferManager Dedup | ✅ | ✅ | ✅ | |
| Transfer State Listener | ✅ | 🟡 | 🟡 | channel vs state mutation |
| Object Added Listener | ✅ | ❌ | ❌ | |
| Capture Complete Listener | ✅ | ❌ | ❌ | |
| Store Removed Listener | ✅ | ❌ | ❌ | |
| Auto-Transfer Setting Persist | ✅ | — | ✅ | |
| Transfer Notification Dialog | ✅ | — | ✅ | |
| Enable/Disable Event Polling | ✅ | ❌ | ❌ | |
| Event Array Parsing | ✅ | 🟡 | 🟡 | framed but not dispatched |
| Remote Mode State Tracking | ✅ | ❌ | ❌ | |
| Liveview State Sync w/ Events | — | — | — | n/a |

### Photo Browse & Thumbnails

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| GetObjectHandles | ✅ | ✅ | ✅ | |
| GetNumObjects | ✅ | ❌ | 🟡 | mobile const unused |
| GetObjectInfo | ✅ | ✅ | ✅ | |
| GetThumbnail | ✅ | ✅ | ✅ | |
| GetObject | ✅ | ✅ | ✅ | |
| Object Handle Format Decode | ✅ | ❌ | ❌ | handles opaque |
| Folder Navigation/Hierarchy | ✅ | ❌ | ❌ | flat listing |
| ObjectFormatCode Handling | ✅ | ✅ | ✅ | |
| Supported Format Recognition | ✅ | ✅ | ✅ | |
| EXIF Thumbnail Extraction | ✅ | 🟡 | 🟡 | PTP thumb only |
| Large Thumb from JPEG APP2 MPF | ✅ | ❌ | ❌ | |
| RAW Large Thumb (TIFF SubIFD) | ✅ | ❌ | ❌ | |
| In-Memory Bitmap Caching | ✅ | ✅ | ✅ | CLI LRU 128; mobile Map |
| Smart Down-sampling | ✅ | ❌ | ❌ | |
| EXIF Rotation Handling | ✅ | ❌ | ❌ | |
| Storage Info Queries | ✅ | ✅ | 🟡 | mobile const unused |
| Multi-Storage/Dual-Slot | ✅ | 🟡 | ❌ | |
| Folder List Caching/session | ✅ | ❌ | ❌ | |
| Thumbnail Cache Path Mgmt | ✅ | ❌ | ❌ | |
| GetSpecificSizeObject (Nikon) | ✅ | ❌ | ❌ | |
| Image Size Template | ✅ | ❌ | ❌ | |
| Thumbnail Grid Dynamic Sizing | ✅ | 🟡 | ✅ | |
| File Type Icon Overlay | ✅ | ✅ | ✅ | |
| Thumbnail Load Progress | ✅ | 🟡 | ✅ | mobile "Developing X/Y" |
| File Filtering/Search Folder | ✅ | ✅ | ✅ | |
| Dir Depth from Handle | ✅ | ❌ | ❌ | |
| File Rename/Filename Extract | ✅ | 🟡 | ✅ | uses ObjectInfo.Filename |
| Transfer Folder by Date/Mode | ✅ | 🟡 | ❌ | |
| Device Storage Full Check | ✅ | ❌ | ❌ | |
| EXIF Preservation on Download | ✅ | ❌ | ❌ | bytes as-is |
| Session State Management | ✅ | ✅ | ✅ | |
| Thread-Safe Thumb State | ✅ | ✅ | ✅ | |
| Folder Info Callback Workflow | ✅ | ❌ | ✅ | CLI blocking |
| Thumb Cache Persist Cross-Session | ✅ | ❌ | ❌ | |
| GetPartialObject Chunked | ✅ | ✅ | ✅ | |
| Download Resume | ✅ | ✅ | ✅ | |
| Keep-Alive Round-Trip | ✅ | ❌ | ✅ | |
| Conn Bind to Net Interface | ✅ | ❌ | ✅ | |
| Nikon GetLargeThumb Fallback | ✅ | ✅ | ✅ | |

### Photo Download & Transfer List

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| GetObject | ✅ | ✅ | ✅ | |
| GetPartialObject | ✅ | ✅ | ✅ | |
| GetTransferList | ✅ | ❌ | ❌ | Picup queue |
| SetTransferListLock | ✅ | ❌ | ❌ | |
| GetObjectInfo | ✅ | ✅ | ✅ | |
| GetObjectHandles | ✅ | ✅ | ✅ | |
| TransferManager Dedup | ✅ | ✅ | ✅ | |
| GetObjectState Offset Tracking | ✅ | ✅ | ✅ | |
| StorageFullCheck | ✅ | ❌ | ❌ | |
| Movie/Video Download | ✅ | ✅ | ✅ | MOV 0x300D |
| RAW File Download | ✅ | 🟡 | 🟡 | no compression filter |
| GetThumbnail | ✅ | ✅ | ✅ | |
| Image Resizing/Downsampling | ✅ | ❌ | ❌ | |
| EXIF Preservation | ✅ | ❌ | ❌ | |
| Save Folder Organization | ✅ | ✅ | 🟡 | mobile flat dir |
| Cache Management | ✅ | ❌ | ❌ | |
| GetObjectErrorState Handling | ✅ | 🟡 | 🟡 | no granular codes |
| Picup Transfer | ✅ | ❌ | ❌ | |
| Compression Format Variants | ✅ | ❌ | ❌ | |
| NotifyFileAcquisitionStart/End | ✅ | ❌ | ❌ | |
| Multi-Format Detection | ✅ | ✅ | ✅ | |
| StorageID/Slot Mgmt | ✅ | ✅ | 🟡 | |
| Folder Handle Hierarchy | ✅ | 🟡 | 🟡 | flat |
| Cancel/Abort Transfer | ✅ | 🟡 | 🟡 | no CancelTransaction |
| Duplicate File Handling (_N) | ✅ | ❌ | ❌ | |
| File Format Conversion | ✅ | ❌ | ❌ | |
| GetCompressionSetting | ✅ | ❌ | ❌ | |
| DscFolderManager | ✅ | ❌ | ❌ | out of scope |
| GetSpecificSizeObject | ✅ | ❌ | ❌ | |
| FileOutputStream Streaming | ✅ | ✅ | ✅ | |
| EXIF from RAW | ✅ | ❌ | ❌ | |
| Connection Keep-Alive | ✅ | 🟡 | ✅ | CLI TCP only |
| Connection Session Mgmt | ✅ | ✅ | ✅ | |
| TCP Socket Tuning | ✅ | ✅ | ❌ | mobile relies on plugin |
| Network Binding (Split Routing) | ✅ | ❌ | ✅ | |
| Reconnection & Auto-Discovery | ✅ | ❌ | ✅ | |
| Event Channel & Loop | ✅ | 🟡 | 🟡 | drained |
| Transaction Serialization | ✅ | ✅ | ✅ | |
| Error/Disconnection Handling | ✅ | 🟡 | 🟡 | no retry |

### Remote Capture

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| Initiate Capture (0x100E) | ✅ | ✅ | ✅ | |
| Bulb Capture (0x1105) | ✅ | ❌ | ❌ | |
| Terminate Capture (0x9105) | ✅ | ❌ | ❌ | |
| AF Drive (0x9125) | ✅ | ❌ | ❌ | |
| AF Drive Cancel | ✅ | ❌ | ❌ | |
| Change AF Area (0x9184) | ✅ | ❌ | ❌ | |
| Change Camera Mode (0x9004) | ✅ | ❌ | ❌ | |
| Remote Mode Start (1101) | ✅ | ❌ | ❌ | |
| Remote Mode End | ✅ | ❌ | ❌ | |
| Liveview Image Acquisition (0x90FB) | ✅ | ❌ | ❌ | |
| Liveview Start | ✅ | ❌ | ❌ | |
| Liveview End (0x90FC) | ✅ | ❌ | ❌ | |
| Liveview-Integrated Capture | ✅ | ❌ | ❌ | |
| Unfocused Capture Support | ✅ | ❌ | ❌ | |
| Out-of-Focus Detection (40962) | ✅ | ❌ | ❌ | |
| Capture Complete Event (0x4009) | ✅ | ❌ | ❌ | |
| Capture Error Handling | ✅ | 🟡 | 🟡 | generic only |
| Self-Timer Capture | ✅ | ❌ | ❌ | |
| Exposure Program Mode Select | ✅ | 🟡 | 🟡 | read-only |
| Extended Exposure Modes (X) | ✅ | ❌ | ❌ | |
| Focus Mode Info | ✅ | 🟡 | 🟡 | raw number |
| Extended Focus Mode (X) | ✅ | ❌ | ❌ | |
| Shutter Speed Readback | ✅ | 🟡 | 🟡 | read-only |
| Exposure Bias Compensation | ✅ | 🟡 | 🟡 | no UI (varies) |
| Aperture (F-Number) Readback | ✅ | 🟡 | 🟡 | liveview-tied in WMU |
| Bulb Mode Support Detection | ✅ | 🟡 | ❌ | |
| HDR Mode Detection | ✅ | ❌ | ❌ | |
| Lens Detection | ✅ | ❌ | ❌ | |
| Retractable Lens Warning | ✅ | ❌ | ❌ | |
| External DC Power Detection | ✅ | ❌ | ❌ | |
| Battery Level Monitoring | ✅ | 🟡 | 🟡 | on-demand |
| Free Space Monitoring | ✅ | ❌ | ❌ | |
| Liveview Countdown Timer | ✅ | ❌ | ❌ | |
| Focus Status Indicator | ✅ | ❌ | ❌ | |
| AF Frame Display | ✅ | ❌ | ❌ | |
| Face Detection AF Display | ✅ | ❌ | ❌ | |
| Focus Point Edit (Touch AF) | ✅ | ❌ | ❌ | |
| User Mode Info | ✅ | ❌ | ❌ | |
| Warning Status Check | ✅ | ❌ | ❌ | |
| Liveview Prohibition Detection | ✅ | ❌ | ❌ | |
| Capture Timeout Calculation | ✅ | 🟡 | ❌ | CLI fixed 30 s |
| Zoom Control | ✅ | ❌ | ❌ | |
| Device Ready Check | ✅ | ❌ | ❌ | |
| Compression Setting Detection | ✅ | 🟡 | ❌ | |
| Capture Mode Lock | ✅ | ❌ | ❌ | |
| Liveview Image Preparation | ✅ | ❌ | ❌ | |
| Capture Completion Notification | ✅ | ❌ | ❌ | |

### Live View

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| Start LV Session | ✅ | ❌ | ❌ | |
| End LV Session | ✅ | ❌ | ❌ | |
| Get LV Image Stream | ✅ | ❌ | ❌ | |
| LV Frame Rate Control | ✅ | ❌ | ❌ | |
| LV AF Area Movement | ✅ | ❌ | ❌ | |
| LV AF Drive | ✅ | ❌ | ❌ | |
| LV AF Frame Visualization | ✅ | ❌ | ❌ | |
| Face AF Detection | ✅ | ❌ | ❌ | |
| LV Zoom Control | ✅ | ❌ | ❌ | |
| LV Battery Monitoring | ✅ | 🟡 | 🟡 | once at startup |
| LV Temperature Monitoring | ✅ | ❌ | ❌ | |
| LV State Machine | ✅ | ❌ | ❌ | |
| LV Capture Integration | ✅ | 🟡 | 🟡 | capture standalone |
| LV Metadata Extraction | ✅ | ❌ | ❌ | |
| LV Prohibition Condition Check | ✅ | ❌ | ❌ | |
| LV Event Handling | ✅ | 🟡 | ❌ | |
| LV Response Code Handling | ✅ | ❌ | ❌ | |
| LV Model-Specific Behavior | ✅ | 🟡 | ❌ | CLI profiles exist |
| LV Display Rendering | ✅ | ❌ | ❌ | |
| LV Free Space Tracking | ✅ | ❌ | ❌ | |
| LV Shutter/F-Number/ISO Metadata | ✅ | ❌ | ❌ | |
| LV AF Practicability / AfAtLiveView / IsoStep | ✅ | ❌ | ❌ | |

### Camera Property Control

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| Exposure Bias Get/Set/Array | ✅ | ✅ | ✅ | `codes.go:203` |
| Exposure Program Mode Get | ✅ | ✅ | ✅ | `0x500E` |
| Exposure Program Mode Get X | ✅ | ❌ | ❌ | |
| Focus Mode Get | ✅ | ❌ | ❌ | mobile const unused |
| Focus Mode Get X | ✅ | ❌ | ❌ | |
| Shutter Speed Get | ✅ | ✅ | ✅ | `0x500D` |
| F-Number Get | ✅ | ✅ | ✅ | `0x5007` |
| ISO Sensitivity Get | ✅ | ✅ | ✅ | `0x500F` |
| HDR Mode Get | ✅ | ❌ | ❌ | |
| Compression Setting Get | ✅ | ❌ | ❌ | |
| Battery Level Get | ✅ | ✅ | 🟡 | not in mobile CONTROL_PROPS |
| External DC In Get | ✅ | ❌ | ❌ | |
| Lens Info Get | ✅ | ❌ | ❌ | |
| User Mode Get | ✅ | ❌ | ❌ | |
| Camera Mode Change | ✅ | 🟡 | 🟡 | via SetDevicePropValue |
| Liveview Prohibition Get | ✅ | ❌ | ❌ | |
| Warning Status Get | ✅ | ❌ | ❌ | |
| Retractable Lens Warning Get | ✅ | ❌ | ❌ | |
| Liveview Image Data Get | ✅ | ❌ | ❌ | |
| Autofocus Drive / Area Change | ✅ | ❌ | ❌ | |
| White Balance Query | ✅ | ✅ | ✅ | `0x5005` |
| Connection Setup & Session | ✅ | ✅ | ✅ | |
| TCP Keep-Alive Tuning | ✅ | ✅ | 🟡 | plugin-dependent |
| Conn Reconnection & Resilience | ✅ | 🟡 | 🟡 | no auto-reconnect |
| Property Read Perf & Batching | ✅ | ✅ | ✅ | |

### GPS / Geotagging (all CLI —, all mobile ❌ unless noted)

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| Location Manager Init | ✅ | — | ❌ | |
| GPS Singleton Manager | ✅ | — | ❌ | |
| Dual-Provider Updates | ✅ | — | ❌ | |
| Location Callback Handler | ✅ | — | ❌ | |
| Provider State Tracking | ✅ | — | ❌ | |
| Last Known Location Fallback | ✅ | — | ❌ | |
| Remove Location Updates | ✅ | — | ❌ | |
| Coord→EXIF Conversion | ✅ | — | ❌ | |
| Lat/Long String Generation | ✅ | — | ❌ | |
| Lat/Long Reference Direction | ✅ | — | ❌ | |
| Altitude Encoding/Reference | ✅ | — | ❌ | |
| GPS Provider Status Check | ✅ | — | ❌ | |
| Location Permission Validation | ✅ | — | 🟡 | perm declared for Wi-Fi scan only |
| EXIF GPS Tagging on Transfer | ✅ | — | ❌ | |
| Empty GPS Placeholder Detection | ✅ | — | ❌ | |
| EXIF GPS Tag Writing | ✅ | — | ❌ | |
| GPS Capture/Transfer/Template Mode | ✅ | — | ❌ | |
| GPS Settings UI + Action Buttons | ✅ | — | ❌ | |
| Capture/Transfer GPS Init & Prompt | ✅ | — | ❌ | |
| EXIF GPS Preservation on Resize | ✅ | — | ❌ | n/a (no resize) |
| RAW File GPS Handling | ✅ | — | ❌ | |
| File Detail Location Display | ✅ | ❌ | ❌ | |
| Detail View GPS Display | ✅ | ❌ | ❌ | |
| Content Resolver Media Index | ✅ | — | ❌ | |
| Activity/Session GPS Cleanup | ✅ | — | ❌ | |
| GPS Data Caching | ✅ | — | ❌ | |

### Settings & Preferences

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| SSID / Wi-Fi Auth / WPA Passphrase Config | ✅ | ❌ | ❌ | camera-side, out of scope |
| Wi-Fi Channel | ✅ | ❌ | ❌ | |
| Idle Timeout | ✅ | 🟡 | 🟡 | hardcoded |
| Wi-Fi Keep-Alive Time | ✅ | 🟡 | 🟡 | not configurable |
| WPS PIN/Mode/Timeout | ✅ | ❌ | ❌ | |
| Subnet/DHCP Server/Client IP | ✅ | ❌ | ❌ | |
| Input Password Format | ✅ | ❌ | ❌ | |
| Camera Serial Number | ✅ | ✅ | ✅ | |
| Camera Firmware Version | ✅ | ✅ | ✅ | |
| Get NCC Settings | ✅ | ❌ | ❌ | |
| Auto Transfer Image | ✅ | ✅ | ✅ | |
| Auto Launch App | ✅ | — | 🟡 | |
| Auto Sync Time | ✅ | ❌ | ❌ | |
| License Agreement / URL | ✅ | ❌ | ❌ | |
| Image Size Template | ✅ | ❌ | ❌ | |
| Battery Status Display | ✅ | ✅ | 🟡 | |
| Thumbnails Per Page | ✅ | ❌ | ❌ | |
| GPS Capture/Transfer/Template | ✅ | — | 🟡/❌ | |
| Language Selection | ✅ | ❌ | ❌ | English only |
| Frame Rate Setting | ✅ | ❌ | ❌ | |
| Capture Mode / Liveview settings | ✅ | ✅/❌ | ✅/❌ | |
| Last Thumbnail Paths / Folder | ✅ | ❌ | ❌ | |
| Wi-Fi Connection History | ✅ | 🟡 | 🟡 | 4 SavedCameras (CLI) |
| Device Info Retrieval | ✅ | ✅ | ✅ | |
| Settings Persistence / Init | ✅ | ✅ | ✅ | |
| Context/Device Metadata | ✅ | 🟡 | 🟡 | |
| Debug Mode / Liveview Debug | ✅ | ❌ | ❌ | |

### Image Processing

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| EXIF Tag Parsing (IFD0) | ✅ | 🟡 | 🟡 | PTP ObjectInfo only |
| EXIF IFD1 Thumbnail Reading | ✅ | ❌ | ❌ | |
| EXIF IFD2 RAW Metadata | ✅ | ❌ | ❌ | |
| EXIF Pointer Navigation | ✅ | ❌ | ❌ | |
| EXIF DateTime/CaptureDate | ✅ | ✅ | ✅ | `ParsePTPDateTime` |
| EXIF Camera Metadata (Make/Model/Flash/Focal) | ✅ | ❌ | ❌ | |
| Image Orientation Tag | ✅ | ❌ | ❌ | |
| JPEG MPF Parsing | ✅ | ❌ | ❌ | |
| JPEG Thumb from MPF | ✅ | ❌ | ❌ | |
| Small Thumb (EXIF standard) | ✅ | 🟡 | 🟡 | no size validation |
| Bitmap In-Sample Resize | ✅ | ❌ | ❌ | |
| Bitmap Resize (quality loss) | ✅ | 🟡 | ❌ | CLI upscale only |
| Bitmap Rotation (90°) | ✅ | ❌ | ❌ | |
| Image Cache (memory pool) | ✅ | ✅ | 🟡 | mobile no cache |
| EXIF Rotation Mod (in-place) | ✅ | ❌ | ❌ | |
| RAW EXIF Extraction | ✅ | ❌ | ❌ | |
| RAW→JPEG Conversion | ✅ | ❌ | ❌ | |
| JPEG Recompress w/ EXIF Preserve | ✅ | ❌ | ❌ | |
| Compression Setting Detection | ✅ | ❌ | ❌ | |
| Object Format Code ID | ✅ | ✅ | ✅ | |
| Object Handle Format Decode | ✅ | ❌ | ❌ | not needed |
| Image Type Classification | ✅ | ✅ | ✅ | |
| Slot2 Save Mode Detection | ✅ | ❌ | ❌ | |
| GetPartialObject | ✅ | ✅ | ✅ | |
| GetObject | ✅ | ✅ | ✅ | |
| GetThumbnail | ✅ | ✅ | ✅ | |
| GetSpecificSizeObject | ✅ | ❌ | ❌ | |
| Object Handle Enumeration | ✅ | ✅ | ✅ | |
| Object Info Dataset | ✅ | ✅ | ✅ | |
| LiveView Image Download | ✅ | ❌ | ❌ | |
| NotifyFileAcquisition | ✅ | ❌ | ❌ | |
| Transfer List Tracking | ✅ | ❌ | ✅ | mobile db |
| Image Dimension Reporting | ✅ | ✅ | ✅ | |
| Thumbnail Pixel Dim Tags | ✅ | 🟡 | 🟡 | parsed not exposed |
| Bitmap Config (ARGB_8888) | ✅ | — | — | n/a |
| Image Transfer Size Templates | ✅ | ❌ | ❌ | |
| Video Format Support (MOV/WAV/AVI) | ✅ | ✅ | ✅ | no video player |

### UI Screens & App Flow

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| Startup w/ Permission Mgmt | ✅ | 🟡 | 🟡 | |
| NFC Wi-Fi Config Flow | ✅ | ❌ | ❌ | |
| PTP Conn Open/Close Lifecycle | ✅ | ✅ | ✅ | |
| Top-Level Menu | ✅ | ✅ | ✅ | |
| Browse w/ Dual Source Menu | ✅ | ❌ | 🟡 | camera-only |
| Picture Up (PICUP) Mode | ✅ | ❌ | ❌ | |
| Remote/Camera Mode Toggle | ✅ | 🟡 | 🟡 | |
| Live View Display + AF Frame | ✅ | ❌ | ❌ | |
| Live View Telemetry Overlay | ✅ | ❌ | ❌ | |
| Bulb Mode UI | ✅ | 🟡 | ❌ | |
| Self-Timer Countdown | ✅ | ❌ | ❌ | |
| Exposure Bias Seek Bar | ✅ | 🟡 | 🟡 | mobile stepper |
| Zoom Control Buttons | ✅ | ❌ | ❌ | |
| AF Trigger & Area Select | ✅ | ❌ | ❌ | |
| Capture Complete Auto-Transfer | ✅ | ❌ | 🟡 | mobile polls |
| Object Added Listener | ✅ | ❌ | ❌ | |
| Transfer Progress Dialog | ✅ | 🟡 | ✅ | |
| Transfer Error Dialogs | ✅ | 🟡 | 🟡 | |
| Thumbnail Grid/List | ✅ | ✅ | ✅ | |
| DSC Thumbnail (Camera) | ✅ | ✅ | ✅ | |
| Folder Nav (Device/Camera) | ✅ | ❌ | ❌ | |
| Image Detail w/ EXIF | ✅ | ❌ | 🟡 | basic ObjectInfo |
| Image Viewer (Full-Screen) | ✅ | ❌ | 🟡 | mobile pinch-zoom |
| Settings Menu | ✅ | 🟡 | 🟡 | no camera Wi-Fi editor |
| Device Battery Display | ✅ | ✅ | 🟡 | mobile hardcoded 84% |
| Wi-Fi Conn Status Indicator | ✅ | 🟡 | ✅ | |
| Background Service | ✅ | — | ❌ | |
| Connection Status Notifications | ✅ | — | ❌ | |
| Screen State Monitoring | ✅ | — | 🟡 | no pause handler |
| Wi-Fi State Monitoring | ✅ | 🟡 | 🟡 | |
| User Session Monitoring | ✅ | — | ❌ | |
| Startup Boot Receiver | ✅ | — | — | |
| Dialog Message Handler | ✅ | 🟡 | ✅ | toasts only |
| Bulk Action Confirmation | ✅ | ❌ | ❌ | |
| Session Mgmt (Open/Close) | ✅ | ✅ | ✅ | |
| Event Command Polling Loop | ✅ | ❌ | ❌ | events discarded |
| Command Queue Sync | ✅ | ✅ | ✅ | |
| Remote Mode Start/End | ✅ | ❌ | ❌ | |
| Storage Detection & Capacity | ✅ | ✅ | ❌ | |
| Device Temperature Monitoring | ✅ | ❌ | ❌ | |
| Lens Detection & Warning | ✅ | ❌ | ❌ | |
| HDR Mode Incompatibility Check | ✅ | ❌ | ❌ | |
| Camera Mode Availability Checks | ✅ | 🟡 | 🟡 | |
| Panorama Mode Handling | ✅ | ❌ | ❌ | |
| Image Format/Compression Select | ✅ | 🟡 | ❌ | |
| Image Resize on Transfer | ✅ | ❌ | ❌ | |
| GPS Metadata Embedding | ✅ | ❌ | ❌ | |
| Debug Mode Display | ✅ | 🟡 | ❌ | |
| Activity Pause/Resume on Lock | ✅ | — | 🟡 | no pause disconnect |
| Config Change (Rotation) | ✅ | — | 🟡 | CSS reflow |
| App Locale Switching | ✅ | ❌ | ❌ | |
| License Activity (EULA/OSS) | ✅ | ❌ | ❌ | |
| Thumbnail Image Caching | ✅ | ✅ | ❌ | |
| Error Handler Callback Chain | ✅ | 🟡 | 🟡 | |
| Multi-Touch Barrier | ✅ | — | ❌ | |
| Date/Time Sync | ✅ | ❌ | ❌ | |
| Camera/Factory Reset | ✅ | ❌ | ❌ | |
| Object Handle Mgmt/Tracking | ✅ | 🟡 | ✅ | |
| Probe Request (Wi-Fi Discovery) | ✅ | ✅ | 🟡 | mobile full-connect probe |
| TCP Window Buffer Optimization | ✅ | ✅ | ✅ | |
| Keep-Alive Mechanism | ✅ | ❌ | ✅ | CLI TCP-only |

### Native Library Surface (PTP/PTPIP Stack)

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| OpenPTPIP / ClosePTPIP | ✅ | ✅ | ✅ | |
| OpenSession / CloseSession | ✅ | ✅ | ✅ | |
| SetTcpWindowClampRecvBuf | ✅ | 🟡 | ❌ | CLI 4 MiB fixed, no clamp |
| ProbeRequest NCC Keep-Alive | ✅ | 🟡 | ❌ | passive only |
| GetEvent / GetEventCommand | ✅ | ❌ | ❌ | drained |
| GetDeviceInfo | ✅ | ✅ | ❌ | mobile uses responderName |
| PTPIP Init Handshake | ✅ | ✅ | ✅ | |
| GetStorageIDs | ✅ | ✅ | ✅ | |
| GetStorageInfo | ✅ | ✅ | ❌ | |
| GetNumObjects | ✅ | ❌ | ❌ | |
| GetObjectHandles / Info / Object | ✅ | ✅ | ✅ | |
| GetPartialObject | ✅ | ✅ | ✅ | |
| GetThumbnail | ✅ | ✅ | ✅ | |
| GetSpecificSizeObject | ✅ | ❌ | ❌ | |
| Capture (0x100E) | ✅ | ✅ | ✅ | no compression/burst params |
| TerminateCapture (0x1011) | ✅ | ❌ | ❌ | |
| Start/Get/End/Prepare Liveview | ✅ | ❌ | ❌ | |
| AfDrive / ChangeAfArea / Zoom | ✅ | ❌ | ❌ | |
| GetExposureProgramMode | ✅ | 🟡 | 🟡 | raw value, no enum decode |
| GetFocusMode | ✅ | ❌ | 🟡 | |
| GetShutterSpeed | ✅ | 🟡 | ✅ | |
| Get/Set ExposureBias | ✅ | ✅ | ✅ | |
| GetCompressionSetting | ✅ | ❌ | ❌ | |
| GetBatteryLevel | ✅ | ✅ | ❌ | |
| GetWarningStatus / HDR / LensSort / Retractable / LV Prohibition / ExternalDcIn / UserMode | ✅ | ❌ | ❌ | |
| GetX* Extended Modes | ✅ | ❌ | ❌ | |
| ChangeCameraMode / DeviceReady | ✅ | ❌ | ❌ | |
| SetDateTime | ✅ | ❌ | ❌ | |
| NotifyFileAcquisition Start/End | ✅ | ❌ | ❌ | |
| Set/GetTransferList[Lock] | ✅ | ❌ | ❌ | |
| GetSlot2ImageSaveMode | ✅ | ❌ | ❌ | |
| All SetNcc* / GetNccSettings / NccFactoryReset / NccResetDevice / NccGetDeviceInfo / NccGetLog / NccGet/SetInfo / NccGet/SetPropValue / NccGetDeviceTemperture | ✅ | ❌ | ❌ | camera provisioning, out of scope |
| PTP Generic Data Marshaling | ✅ | ✅ | ✅ | |
| PTPIP Cmd/Evt Socket Comm | ✅ | ✅ | ✅ | |
| TCP Socket Options Config | ✅ | 🟡 | ❌ | |
| PTPIP Version Negotiation | ✅ | ✅ | ✅ | |
| PTP Response Code Handling | ✅ | ✅ | 🟡 | mobile missing 8+ codes |
| PTP Event Code Enum | ✅ | ❌ | ❌ | |
| PTP Property Code Registry | ✅ | 🟡 | 🟡 | vendor props missing |
| Multi-threaded Event Processing | ✅ | 🟡 | ✅ | Go doesn't expose `Events()` |
| JNI Logging | — | — | — | n/a |

### Error Handling, Logging, Licensing

| Feature | WMU | CLI | Mobile | Notes |
|---|---|---|---|---|
| PTP Error Codes (Network/IP) | ✅ | 🟡 | 🟡 | no socket/timeout taxonomy |
| PTP Error Codes (USB) | — | — | — | Wi-Fi only |
| PTP Error Codes (System Resource) | ✅ | ❌ | ❌ | |
| PTP Error Codes (Socket Factory) | ✅ | — | 🟡 | |
| PTP Response Codes (Device) | ✅ | 🟡 | 🟡 | subset |
| PTP Response Codes (Nikon Specific) | ✅ | ❌ | ❌ | |
| Socket Connection Error Handling | ✅ | ✅ | ✅ | no granular classification |
| Logger Facade | ✅ | 🟡 | 🟡 | no levels |
| Native Logging (Android Log) | ✅ | — | 🟡 | |
| Native Error Code Resolution | ✅ | 🟡 | ❌ | |
| Device Ready Status Check | ✅ | ❌ | ❌ | |
| PTP Session Management | ✅ | ✅ | ✅ | no re-open recovery |
| PTP/IP Connection Management | ✅ | ✅ | ✅ | no reconnect/heartbeat |
| Event Retrieval with Timeout | ✅ | 🟡 | ❌ | |
| Transfer List Management | ✅ | ❌ | ❌ | |
| Device Info Retry Logic | ✅ | ❌ | ❌ | called once |
| GetSpecificSizeObject Retry State | ✅ | ❌ | 🟡 | |
| Response Code Validation | ✅ | ✅ | ✅ | no recoverable-code handling |
| LiveView Error Handling | — | — | — | n/a |
| Notification Controller | ✅ | — | 🟡 | no conn notifications |
| AssistService Foreground Notif | ✅ | — | ❌ | |
| EULA / License gating / URL / Debug / NFC | ✅ | ❌ | ❌ | open source, mostly n/a |
| Debug Mode Manager | ✅ | ❌ | ❌ | |
| NCC/Android/Logcat Log Extraction | ✅ | ❌/— | ❌ | |
| PTP Protocol Error Diagnostics | ✅ | 🟡 | 🟡 | no opcode/txn context |
| PTPIP Connection Diagnostics | ✅ | ❌ | ❌ | no staged handshake trace |
| Timeout Config (Idle/WPS) | ✅ | ❌ | ❌ | hardcoded |
| Battery Status Monitoring | ✅ | 🟡 | 🟡 | no low-battery warning |
| Capture Timeout Per Model | ✅ | ❌ | ❌ | |
| Exception Handling (General) | ✅ | 🟡 | 🟡 | |
| Stack Guard/Security | ✅ | 🟡 | 🟡 | runtime-delegated |
| Keep-Alive Socket Config | ✅ | ✅ | 🟡 | CLI 15 s period |
| TCP Receive Buffer Tuning | ✅ | ✅ | 🟡 | |
| Transaction Serialization Lock | ✅ | ✅ | ✅ | |

---

## 3. Consistent Connection Management — Deep Dive

This pulls together every `connectionRelated` feature across all domains and assesses robustness and CLI↔mobile consistency.

### 3.1 What is solid and consistent on both clients
- **Handshake & dual-socket transport.** Both open command + event TCP connections on port 15740, perform `InitCommandRequest`/`InitEventRequest` with the fixed WMU initiator GUID, negotiate `ProtocolVersion10`, and open a session (`client.go:70-119`, `client.ts:114-183`). This is faithful to WMU and is the strongest area of parity.
- **Session lifecycle.** `OpenSession`/`CloseSession` are correct on both; both treat `SessionAlreadyOpen` as success on reconnect (`client.go:110-117`, `client.ts:379`).
- **Transaction serialization.** Go uses `sync.Mutex` on `Transact()`; mobile uses a promise-chain `txnQueue` (`client.ts:94-101`). Only one PTP transaction in flight at a time on both — matching WMU and preventing interleave corruption.
- **TCP window/throughput tuning** is present on both at some layer (CLI explicitly `SetReadBuffer(4 MiB)` + `TCP_NODELAY`; mobile via native plugin).

### 3.2 The core inconsistencies between CLI and mobile

This is the headline finding of the connection deep-dive: **the two clients have materially different connection-robustness postures.**

| Capability | CLI | Mobile | Consequence |
|---|---|---|---|
| App-level keep-alive | ❌ TCP `SetKeepAlivePeriod(15s)` only (`client.go:128`) | ✅ 9 s `GetStorageIDs` ping (`store.ts:704-726`) | CLI is far more likely to hit D5300-class idle drops during long browse/idle. This is the single biggest inconsistency. |
| Wi-Fi interface binding | ❌ uses default route | ✅ `bindWifi=true`, `acquireWifiNetwork` (`TcpSocketPlugin.kt:258-300`) | CLI may route camera traffic over Ethernet/VPN on multi-homed hosts. |
| Auto-discovery / reconnect | ❌ manual IP | ✅ `autoConnect()` races candidates w/ + w/o binding (`store.ts:297-388`) | CLI requires re-run after any drop. |
| `GetDeviceInfo` in connect path | ✅ full DeviceInfo parsed | ❌ only `responderName` from InitCommandAck | Mobile lacks supported-ops/props list, so it can't feature-gate by model. |
| Socket tuning surface | ✅ explicit | 🟡 plugin-internal, no `SO_RCVBUF`/`TCP_WINDOW_CLAMP` API | Mobile can't tune per-connection. |

### 3.3 Robustness gaps common to BOTH (vs WMU)
1. **No proactive `ProbeRequest`.** Both only *respond* to camera probes (`client.go:328-330`); neither sends the periodic `ProbeRequest` WMU fires every ~20 s. Mobile's `GetStorageIDs` ping is a functional substitute; the CLI has nothing at the app layer.
2. **No background disconnect detection.** Errors surface only on the *next* operation (`dropLocked` in `client.go:336-345`; pending-reject on socket error in `client.ts:130-134`). WMU sets a `dscDetectDisconnected` flag synchronously. Neither client notifies the app the instant the socket dies.
3. **No auto-reconnect with backoff.** Both bail to the app on error. WMU-parity would need exponential backoff + session re-open.
4. **No session/connection state machine.** Both use a binary connected/`sessionOpen` flag. There's no `disconnected → init-cmd → init-evt → session-open → ready` progression, which makes staged recovery and diagnostics impossible.
5. **Event channel is opened but dead.** Both drain and discard events, so connection-relevant events (`StoreRemoved`, `StoreFull`, `DevicePropChanged` for DC-in) never reach the app. The event connection is currently pure liability (kept alive but unused).
6. **Timeouts don't match WMU and aren't configurable.** CLI dial 5 s / io 30 s; mobile connect 8 s / txn 15 s. WMU is 10 s connect. None are user-adjustable, and capture timeout isn't computed from shutter speed (CLI fixed 30 s; mobile fixed 15 s).
7. **No staged handshake diagnostics** — a failed connect yields "dial command connection: <err>" with no indication of which handshake stage failed.

### 3.4 Concrete connection improvements (consolidated)
- Unify keep-alive: give the CLI the same app-level ping the mobile has, and make the interval configurable on both (WMU's 30/60/120/300 s options).
- Add a background socket-error listener that fires a disconnect callback immediately, rather than on next op.
- Add auto-reconnect with exponential backoff behind a callback/flag on both clients.
- Introduce an explicit connection/session state enum shared conceptually across both codebases.
- Bring the CLI up to mobile on discovery (subnet probe) and, where OS permits, interface binding.
- Bring mobile up to the CLI on `GetDeviceInfo` retrieval and explicit socket-option control.
- Align dial timeout to 10 s and make transaction/dial timeouts configurable; compute capture timeout from shutter speed.

---

## 4. Prioritized Fix List

### HIGH priority

1. **Add app-level keep-alive to the Go CLI.** Today the CLI relies only on TCP `SetKeepAlivePeriod(15s)` and will idle-drop on D5300-class bodies. Add a periodic cheap PTP round-trip (e.g., `GetStorageIDs`) with an error-triggered disconnect callback. Touch: `internal/ptpip/client.go` (new `KeepAlive()`/timer), `internal/frontend/model.go` (start/stop on idle).

2. **Implement the GetEvent loop and dispatch events (both clients).** Stop discarding events. Parse the event array and dispatch to handlers. Touch: `internal/ptpip/client.go:170-191` (expose `Events()`/callback + `EventCode` enum in `internal/ptpip/codes.go`); `mobile/src/lib/ptpip/client.ts:192-210` (event listener + `EventCode` in `mobile/src/lib/ptpip/constants.ts`).

3. **Wire ObjectAdded (0x4002) → instant auto-import (both clients).** Replace 5 s polling with event-driven queueing. Touch: `mobile/src/store.ts:680-695` (`startAutoImport`), `mobile/src/lib/camera.ts`; CLI `internal/importer/importer.go` + frontend.

4. **Background disconnect detection + callback (both clients).** Fire on socket error immediately, not on next op. Touch: `internal/ptpip/client.go:336-345` (emit on `dropLocked`); `mobile/src/lib/ptpip/client.ts:130-134` + `mobile/src/store.ts:711-724`.

5. **Auto-reconnect with exponential backoff (both clients).** On disconnect/keep-alive failure, retry (1/2/4…30 s). Touch: `mobile/src/store.ts:297-388` (`autoConnect`) + keep-alive error path; CLI new reconnect loop around `internal/ptpip/client.go` Connect.

6. **Add Wi-Fi network auto-recovery on mobile.** On keep-alive/Wi-Fi-drop error, re-run `autoConnect()` with backoff instead of just disconnecting. Touch: `mobile/src/store.ts:704-726`, `mobile/src/lib/wifi.ts`.

7. **Bulb capture + TerminateCapture (0x1105 / 0x9105) — both clients.** Add opcodes and capture-state tracking. Touch: `internal/ptpip/codes.go`, `internal/ptpip/client.go`; `mobile/src/lib/ptpip/constants.ts`, `mobile/src/lib/ptpip/client.ts`.

8. **Proactive `ProbeRequest` keep-alive at the PTP/IP layer (both clients).** Match WMU's ~20 s probe on a timer. Touch: `internal/ptpip/client.go:328-330`; mobile connect() add `keepAliveMode`.

9. **Map socket-layer errors to a PTP error taxonomy (both clients).** Distinguish dial/timeout/EOF/refused. Touch: `internal/ptpip/codes.go` + wrap `net.OpError`/`context.DeadlineExceeded`; `mobile/plugins/tcp-socket` (emit structured codes) + `mobile/src/lib/ptpip/client.ts`.

10. **PTP session re-open recovery (both clients).** On `SessionNotOpen`, auto re-open rather than fail. Touch: `internal/ptpip/client.go:110-119`, `mobile/src/lib/ptpip/client.ts:377-388`.

*(High-priority items also flagged but folded into the above or explicitly out-of-scope: Live View start/end/image/AF-drive/AF-area, GPS LocationManager/singleton/callbacks/EXIF tagging, NFC Wi-Fi flow, camera NCC SSID/auth provisioning, background service. These are large net-new subsystems; treat as separate epics, not quick fixes.)*

### MEDIUM priority

11. **Align connection timeout to 10 s and make timeouts configurable.** CLI 5 s→10 s dial; expose txn/dial timeouts in config. Touch: `internal/ptpip/client.go:34`, `internal/config/config.go`; `mobile/plugins/.../TcpSocketPlugin.kt:110`, `mobile/src/lib/settings.ts`.

12. **Move `GetDeviceInfo` into the connect path with 3–5 retry (both clients).** Touch: `internal/camera/camera.go:78-96`; add `getDeviceInfo()` to `mobile/src/lib/ptpip/client.ts` + `mobile/src/lib/camera.ts`.

13. **Implement `GetStorageInfo` + free-space pre-check before auto-import (both clients).** Touch: add `getStorageInfo()` to `mobile/src/lib/ptpip/client.ts` (const exists, unused); wire `StorageFullCheck()` in `internal/importer/importer.go` and `mobile/src/lib/camera.ts:176-193`.

14. **Add `StoreRemoved` (0x4009) + `StoreFull` (0x4012) event handling (both).** Alert/abort in-flight transfers. Depends on #2. Touch: `internal/ptpip/codes.go`+client; `mobile/src/lib/ptpip/constants.ts`+client.

15. **CaptureComplete (0x400d) listener (both).** Replace the 2 s post-capture poll. Touch: `mobile/src/store.ts:651`, `mobile/src/lib/camera.ts:157-159`; CLI frontend + client.

16. **`GetCompressionSetting` (0x501B) + RAW/RAW+JPEG filtering (both).** Touch: `internal/ptpip/codes.go` + `internal/camera/camera.go:165`; `mobile/src/lib/ptpip/constants.ts` + `mobile/src/lib/camera.ts`.

17. **Mobile: TCP socket tuning parity.** Expose/verify `TCP_NODELAY`, `SO_RCVBUF=4 MiB`, `SO_KEEPALIVE` in the plugin (CLI already has these at `client.go:125-131`). Touch: `mobile/plugins/tcp-socket/android/.../TcpSocketPlugin.kt`, iOS `TcpSocketPlugin.swift`.

18. **CLI: Wi-Fi interface binding option.** Add `--bind-wifi` and bind to `en0`/`wlan0`. Touch: `internal/ptpip/client.go` (custom dialer), `internal/config/config.go`.

19. **CLI: auto-discovery/subnet scan flag.** Mirror mobile discovery. Touch: `internal/camera/scan.go`, main entry.

20. **Capture timeout computed from shutter speed (both).** bulb=30 s, else 2×shutter+5 s, min 30 s. Touch: `internal/ptpip/client.go:35-37` + `internal/camera/settings.go`; `mobile/src/lib/ptpip/client.ts:93-101`.

21. **PTP protocol error diagnostics with opcode/txn/session context (both).** e.g. `OpGetObject (0x1009) txn#5: timeout after 30s`. Touch: `internal/ptpip/client.go`, `mobile/src/lib/ptpip/client.ts:336`.

22. **Mobile: live battery polling + display real value (not hardcoded 84%).** Add `batteryLevel()` to `mobile/src/lib/ptpip/client.ts`, poll in `store.ts`, render in `GalleryScreen.tsx:109-111`; add `BatteryLevel` to `CONTROL_PROPS` in `mobile/src/lib/camera.ts:40-47`.

23. **Mobile: pause/disconnect on app background + connect/disconnect notifications.** Touch: `mobile/src/store.ts` (App `pause` listener → `stopKeepAlive`/`disconnect`), `mobile/src/lib/notifications.ts`.

24. **Expand mobile PTP response codes + response-code→name mapping (both where missing).** Mobile is missing 8+ codes. Touch: `mobile/src/lib/ptpip/constants.ts:51-60`, error paths in `client.ts`.

25. **EXIF binary parser (IFD0 + Orientation) shared groundwork.** Enables orientation-correct display and Make/Model/Focal in detail view. Touch: new `internal/exif/parser.go`, new `mobile/src/lib/exif/parser.ts`; consume in `DetailScreen.tsx` and CLI `render.go`.

26. **Add structured logging with levels + `--verbose` (both).** Touch: `internal/` (slog) + `main.go:28`; `mobile/src/` logger.

27. **CLI incremental listing progress callback.** Match mobile's per-8 progress. Touch: `internal/camera/camera.go` `ListFiles()` + frontend model.

28. **Mobile: true lightweight ProbeRequest discovery** (instead of full-handshake probe). Touch: `mobile/src/lib/discovery.ts`, `mobile/src/lib/ptpip/client.ts`.

29. **Mobile thumbnail LRU cache.** CLI already has one (`internal/thumbnail/thumbnail.go`); mobile re-fetches on scroll-back. Touch: `mobile/src/lib/camera.ts` / `Thumbnail.tsx`.

---

## 5. What aero-shutter Already Does BETTER than WMU

These are places where aero-shutter's design is a genuine improvement over, or a cleaner modern equivalent of, WMU:

1. **Resumable, chunked downloads with offset tracking.** Both clients read the partial `.part`/file size and resume from that offset via `GetPartialObject` (`internal/importer/importer.go:249-256`, `mobile/src/lib/camera.ts:176-179`). This is more robust for flaky Wi-Fi than a naive full re-fetch.

2. **Cross-platform reach.** A single PTP/IP core is realized as both a headless Go CLI (Linux/macOS terminal, with Kitty/iTerm2 inline-image thumbnails) and a Capacitor mobile app — WMU is Android-only.

3. **Persistent import deduplication ledger.** `internal/database/database.go` and `mobile/src/lib/db.ts` key on filename+size to skip already-imported files across sessions, cleanly separating "new" vs "imported" in the gallery.

4. **Modern concurrency instead of thread soup.** Go goroutine + mutex-serialized `Transact()`, and mobile's promise-chain `txnQueue`, replace WMU's `ThreadEvent`/`ThreadProbeRequest` monitors with simpler, race-safe primitives.

5. **`NikonGetLargeThumb (0x90C4)` with graceful fallback to `GetThumb (0x100A)`** on both clients — best-quality thumbnail with a safe path for bodies that don't support the vendor op.

6. **Split-routing / Wi-Fi binding on mobile** (`keepInternetOnCellular`, `bindWifi=true`, `acquireWifiNetwork` without `NET_CAPABILITY_INTERNET`) lets the phone keep cellular internet while talking to the camera AP — smoother than WMU's all-or-nothing Wi-Fi takeover.

7. **Auto-connect that races candidate hosts with and without Wi-Fi binding** (`store.ts:297-388`) — resilient discovery WMU does not expose.

8. **Explicit CLI TCP tuning** (`SetNoDelay`, `SetReadBuffer(4 MiB)`, `SetKeepAlive`) surfaced in code rather than hidden in a native blob (`client.go:121-132`).

9. **Application-level keep-alive as a superior alternative to camera-side idle-timeout config** — rather than depending on `SetNccWifiKeepAliveTime`, mobile actively pings, which is more reliable across firmware variants (though the CLI still needs this — see fix #1).

10. **Deliberate, honest scoping.** By skipping Live View and camera NCC provisioning, aero-shutter keeps the codebase focused on reliable ingest + basic capture — the paths that matter most for a wireless import tool — rather than reproducing WMU's rarely-used camera-config screens.