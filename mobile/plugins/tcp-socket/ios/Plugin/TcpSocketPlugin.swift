import Foundation
import Capacitor
import Network

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

        let params = NWParameters.tcp
        if let tcpOptions = params.defaultProtocolStack.internetProtocol as? NWProtocolTCP.Options {
            tcpOptions.noDelay = true
            tcpOptions.connectionTimeout = Int(timeoutMs / 1000)
        }

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
                    call.resolve(["socketId": socketId])
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

    private func receive(socketId: String, connection: NWConnection) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 8 * 1024) { [weak self] data, _, isComplete, error in
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
