import Foundation
import Capacitor
import Network
import NetworkExtension
import SystemConfiguration.CaptiveNetwork

/**
 * Raw TCP socket bridge for PTP/IP camera connections on iOS.
 *
 * Uses Network.framework's NWConnection. Each connection streams reads and
 * emits them as `data` events (base64); `closed` and `error` events signal
 * lifecycle changes. Writes are sent on the connection's queue.
 */
@objc(TcpSocketPlugin)
public class TcpSocketPlugin: CAPPlugin {

    private var connections: [String: NWConnection] = [:]
    private var counter: Int = 0
    private let lock = NSLock()

    @objc func connect(_ call: CAPPluginCall) {
        guard let host = call.getString("host"),
              let port = call.getInt("port") else {
            call.reject("host and port are required")
            return
        }
        let timeoutMs = call.getInt("timeoutMs") ?? 8000

        lock.lock()
        counter += 1
        let socketId = "sock-\(counter)"
        lock.unlock()

        let nwHost = NWEndpoint.Host(host)
        guard let nwPort = NWEndpoint.Port(rawValue: UInt16(port)) else {
            call.reject("invalid port")
            return
        }

        // bindWifi: on iOS we can PIN the camera socket to the Wi-Fi interface
        // (requiredInterfaceType = .wifi), but that is the extent of what the OS
        // allows. Unlike Android, iOS does NOT let a third-party app keep
        // internet on cellular for the rest of the app while bound to a
        // no-internet Wi-Fi for arbitrary TCP — the system owns internet
        // routing. So this pins the camera path to Wi-Fi (so the camera is
        // reachable even if Wi-Fi Assist would otherwise prefer cellular) but
        // does NOT create a true split. getNetworkCapabilities() reports
        // isSplitRoutingSupported = false so the UI can show the honest message.
        let bindWifi = call.getBool("bindWifi") ?? true

        let params = NWParameters.tcp
        if let tcpOptions = params.defaultProtocolStack.internetProtocol as? NWProtocolTCP.Options {
            tcpOptions.noDelay = true
            tcpOptions.connectionTimeout = Int(timeoutMs / 1000)
        }
        if bindWifi {
            params.requiredInterfaceType = .wifi
        }

        let networkBinding = bindWifi ? "wifi-pinned" : "default"
        let connection = NWConnection(host: nwHost, port: nwPort, using: params)
        let queue = DispatchQueue(label: "tcpsocket.\(socketId)")

        var didResolve = false
        connection.stateUpdateHandler = { [weak self] state in
            guard let self = self else { return }
            switch state {
            case .ready:
                if !didResolve {
                    didResolve = true
                    self.lock.lock()
                    self.connections[socketId] = connection
                    self.lock.unlock()
                    call.resolve(["socketId": socketId, "networkBinding": networkBinding])
                    self.receive(socketId: socketId, connection: connection)
                }
            case .failed(let error):
                if !didResolve {
                    didResolve = true
                    call.reject(error.localizedDescription)
                } else {
                    self.emitError(socketId, error.localizedDescription)
                }
                self.cleanup(socketId)
            case .cancelled:
                self.emitClosed(socketId)
            default:
                break
            }
        }
        connection.start(queue: queue)
    }

    // Read chunk size. Each delivered chunk crosses the native->JS bridge and is
    // base64-encoded, so larger reads mean far fewer (expensive) bridge crossings
    // during a bulk photo transfer. Network.framework auto-tunes the TCP receive
    // window, so this read size is the main iOS throughput lever.
    private static let receiveChunkBytes = 256 * 1024

    private func receive(socketId: String, connection: NWConnection) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: Self.receiveChunkBytes) { [weak self] data, _, isComplete, error in
            guard let self = self else { return }
            if let data = data, !data.isEmpty {
                self.notifyListeners("data", data: [
                    "socketId": socketId,
                    "dataB64": data.base64EncodedString()
                ])
            }
            if let error = error {
                self.emitError(socketId, error.localizedDescription)
                self.cleanup(socketId)
                return
            }
            if isComplete {
                self.emitClosed(socketId)
                self.cleanup(socketId)
                return
            }
            self.receive(socketId: socketId, connection: connection)
        }
    }

    @objc func write(_ call: CAPPluginCall) {
        guard let socketId = call.getString("socketId"),
              let dataB64 = call.getString("dataB64"),
              let data = Data(base64Encoded: dataB64) else {
            call.reject("socketId and dataB64 are required")
            return
        }
        lock.lock()
        let connection = connections[socketId]
        lock.unlock()
        guard let connection = connection else {
            call.reject("Unknown socket \(socketId)")
            return
        }
        connection.send(content: data, completion: .contentProcessed { error in
            if let error = error {
                call.reject(error.localizedDescription)
            } else {
                call.resolve()
            }
        })
    }

    @objc func close(_ call: CAPPluginCall) {
        guard let socketId = call.getString("socketId") else {
            call.reject("socketId is required")
            return
        }
        cleanup(socketId)
        call.resolve()
    }

    @objc func getWifiInfo(_ call: CAPPluginCall) {
        // iOS heavily restricts Wi-Fi introspection; without the (special
        // entitlement) Hotspot / NEHotspotNetwork APIs an app can't read the
        // gateway. Return nulls so JS discovery falls back to the standard
        // Nikon AP candidate list (192.168.1.1, ...).
        call.resolve(["gateway": NSNull(), "ipAddress": NSNull()])
    }

    @objc func getNetworkCapabilities(_ call: CAPPluginCall) {
        // iOS cannot split arbitrary-TCP camera Wi-Fi from cellular internet.
        call.resolve([
            "isSplitRoutingSupported": false,
            "platform": "ios"
        ])
    }

    // MARK: - In-app Wi-Fi joining

    /// Last SSID this app applied via NEHotspotConfiguration, so leaveWifi()
    /// can remove exactly that configuration.
    private var joinedSsid: String?

    /**
     * Join a Wi-Fi network using NEHotspotConfigurationManager.
     *
     * Requires the "Hotspot Configuration" capability
     * (com.apple.developer.networking.HotspotConfiguration entitlement), which
     * must be enabled in the Apple Developer account for a real build.
     *
     * iOS cannot enumerate visible SSIDs for third-party apps, so `ssidPrefix`
     * alone can't be resolved to a concrete SSID — in that case we report
     * joined=false and the UI asks the user for the exact SSID. `bound` is
     * always false: iOS owns network routing and there is no process bind.
     */
    @objc func joinWifi(_ call: CAPPluginCall) {
        let ssid = call.getString("ssid")
        let password = call.getString("password")
        let ssidPrefix = call.getString("ssidPrefix")

        guard let targetSsid = ssid, !targetSsid.isEmpty else {
            // No exact SSID: iOS can't scan by prefix, so tell the caller to ask.
            call.resolve([
                "joined": false,
                "ssid": ssidPrefix ?? "",
                "bound": false
            ])
            return
        }

        let config: NEHotspotConfiguration
        if let password = password, !password.isEmpty {
            config = NEHotspotConfiguration(ssid: targetSsid, passphrase: password, isWEP: false)
        } else {
            // Open network (typical Nikon camera AP).
            config = NEHotspotConfiguration(ssid: targetSsid)
        }
        config.joinOnce = false

        NEHotspotConfigurationManager.shared.apply(config) { [weak self] error in
            if let error = error {
                let nsError = error as NSError
                // "already associated" means we're effectively joined.
                if nsError.domain == NEHotspotConfigurationErrorDomain,
                   nsError.code == NEHotspotConfigurationError.alreadyAssociated.rawValue {
                    self?.joinedSsid = targetSsid
                    call.resolve(["joined": true, "ssid": targetSsid, "bound": false])
                    return
                }
                call.resolve([
                    "joined": false,
                    "ssid": targetSsid,
                    "bound": false
                ])
                return
            }
            self?.joinedSsid = targetSsid
            // apply() succeeding means the config was accepted; verify best-effort.
            self?.fetchCurrentSsid { current in
                let joined = current == nil || current == targetSsid
                call.resolve([
                    "joined": joined,
                    "ssid": targetSsid,
                    "bound": false
                ])
            }
        }
    }

    @objc func currentWifi(_ call: CAPPluginCall) {
        fetchCurrentSsid { ssid in
            call.resolve(["ssid": ssid ?? NSNull()])
        }
    }

    @objc func scanWifi(_ call: CAPPluginCall) {
        // iOS provides no public API for third-party apps to scan SSIDs.
        call.resolve(["networks": []])
    }

    @objc func leaveWifi(_ call: CAPPluginCall) {
        if let ssid = joinedSsid {
            NEHotspotConfigurationManager.shared.removeConfiguration(forSSID: ssid)
            joinedSsid = nil
        }
        call.resolve()
    }

    /// Best-effort current SSID via NEHotspotNetwork.fetchCurrent (iOS 14+),
    /// falling back to CNCopyCurrentNetworkInfo on older systems. Both require
    /// the Hotspot / location entitlements to return a real value.
    private func fetchCurrentSsid(_ completion: @escaping (String?) -> Void) {
        if #available(iOS 14.0, *) {
            NEHotspotNetwork.fetchCurrent { network in
                completion(network?.ssid)
            }
        } else {
            var ssid: String?
            if let interfaces = CNCopySupportedInterfaces() as? [String] {
                for iface in interfaces {
                    if let info = CNCopyCurrentNetworkInfo(iface as CFString) as NSDictionary?,
                       let name = info[kCNNetworkInfoKeySSID as String] as? String {
                        ssid = name
                        break
                    }
                }
            }
            completion(ssid)
        }
    }

    private func cleanup(_ socketId: String) {
        lock.lock()
        let connection = connections.removeValue(forKey: socketId)
        lock.unlock()
        connection?.cancel()
    }

    private func emitClosed(_ socketId: String) {
        notifyListeners("closed", data: ["socketId": socketId])
    }

    private func emitError(_ socketId: String, _ message: String) {
        notifyListeners("error", data: ["socketId": socketId, "message": message])
    }
}
