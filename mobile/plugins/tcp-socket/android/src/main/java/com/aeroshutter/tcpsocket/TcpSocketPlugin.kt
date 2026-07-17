package com.aeroshutter.tcpsocket

import android.content.Context
import android.net.ConnectivityManager
import android.net.DhcpInfo
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.wifi.WifiManager
import android.os.Build
import android.util.Base64
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/**
 * Raw TCP socket bridge for PTP/IP camera connections.
 *
 * Each connection runs on a dedicated background thread that streams 8 KiB
 * reads and emits them as `data` events (base64). `closed` and `error` events
 * signal lifecycle changes. Writes are dispatched to the socket's output stream.
 *
 * Wi-Fi / cellular split (the key feature)
 * ----------------------------------------
 * A Nikon camera's own access point has no internet. Normally, joining it makes
 * the whole phone lose connectivity. To avoid that, when `bindWifi` is set
 * (default true) we ask ConnectivityManager for a Wi-Fi network WITHOUT
 * requiring NET_CAPABILITY_INTERNET and create the camera socket via that
 * network's SocketFactory. Only the camera socket rides Wi-Fi; the process
 * default network is left untouched, so app/system internet keeps flowing over
 * cellular/mobile data. The per-connection NetworkCallback is unregistered on
 * close so we don't hold the Wi-Fi request open.
 */
@CapacitorPlugin(name = "TcpSocket")
class TcpSocketPlugin : Plugin() {

    private data class Conn(
        val socket: Socket,
        val networkCallback: ConnectivityManager.NetworkCallback?,
    )

    private val sockets = ConcurrentHashMap<String, Conn>()
    private val idCounter = AtomicInteger(0)
    private val ioPool = Executors.newCachedThreadPool()

    private fun connectivityManager(): ConnectivityManager =
        context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

    @PluginMethod
    fun connect(call: PluginCall) {
        val host = call.getString("host")
        val port = call.getInt("port")
        val timeoutMs = call.getInt("timeoutMs", 8000)!!
        val bindWifi = call.getBoolean("bindWifi", true)!!
        if (host == null || port == null) {
            call.reject("host and port are required")
            return
        }
        val socketId = "sock-${idCounter.incrementAndGet()}"

        ioPool.execute {
            var networkCallback: ConnectivityManager.NetworkCallback? = null
            try {
                var wifiNetwork: Network? = null
                if (bindWifi) {
                    // Acquire a Wi-Fi network that does NOT require internet, so
                    // we can pin the camera socket to Wi-Fi while the device keeps
                    // cellular internet on the process default network.
                    val acquired = acquireWifiNetwork(timeoutMs)
                    wifiNetwork = acquired?.first
                    networkCallback = acquired?.second
                }

                val socket = if (wifiNetwork != null) {
                    // Route this socket over Wi-Fi explicitly.
                    wifiNetwork.socketFactory.createSocket() as Socket
                } else {
                    Socket()
                }
                socket.tcpNoDelay = true
                socket.connect(InetSocketAddress(host, port), timeoutMs)
                sockets[socketId] = Conn(socket, networkCallback)

                val result = JSObject()
                result.put("socketId", socketId)
                result.put(
                    "networkBinding",
                    if (wifiNetwork != null) "wifi-bound" else "default",
                )
                call.resolve(result)

                readLoop(socketId, socket)
            } catch (e: Exception) {
                // Release the Wi-Fi request if the connect failed.
                if (networkCallback != null) {
                    try {
                        connectivityManager().unregisterNetworkCallback(networkCallback)
                    } catch (_: Exception) {
                    }
                }
                call.reject(e.message ?: "connect failed")
            }
        }
    }

    /**
     * Request a Wi-Fi transport network without NET_CAPABILITY_INTERNET and wait
     * (up to timeoutMs) for it to become available. Returns the Network plus its
     * NetworkCallback (kept so it can be unregistered on close), or null if none
     * arrived — in which case the caller falls back to a default socket.
     */
    private fun acquireWifiNetwork(timeoutMs: Int): Pair<Network, ConnectivityManager.NetworkCallback>? {
        val cm = connectivityManager()
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            // Intentionally NOT requiring NET_CAPABILITY_INTERNET — the camera AP
            // has none, and requiring it would reject the network.
            .build()

        val latch = CountDownLatch(1)
        @Volatile var network: Network? = null
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(available: Network) {
                network = available
                latch.countDown()
            }

            override fun onUnavailable() {
                latch.countDown()
            }
        }

        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                cm.requestNetwork(request, callback, timeoutMs)
            } else {
                cm.requestNetwork(request, callback)
            }
            latch.await(timeoutMs.toLong(), TimeUnit.MILLISECONDS)
        } catch (_: Exception) {
            try {
                cm.unregisterNetworkCallback(callback)
            } catch (_: Exception) {
            }
            return null
        }

        val net = network
        return if (net != null) {
            Pair(net, callback)
        } else {
            try {
                cm.unregisterNetworkCallback(callback)
            } catch (_: Exception) {
            }
            null
        }
    }

    private fun readLoop(socketId: String, socket: Socket) {
        val buffer = ByteArray(8 * 1024)
        try {
            val input = socket.getInputStream()
            while (!socket.isClosed) {
                val n = input.read(buffer)
                if (n < 0) break
                if (n > 0) {
                    val slice = buffer.copyOf(n)
                    val event = JSObject()
                    event.put("socketId", socketId)
                    event.put("dataB64", Base64.encodeToString(slice, Base64.NO_WRAP))
                    notifyListeners("data", event)
                }
            }
            emitClosed(socketId)
        } catch (e: Exception) {
            if (sockets.containsKey(socketId)) emitError(socketId, e.message ?: "read error")
        } finally {
            cleanup(socketId)
        }
    }

    @PluginMethod
    fun write(call: PluginCall) {
        val socketId = call.getString("socketId")
        val dataB64 = call.getString("dataB64")
        if (socketId == null || dataB64 == null) {
            call.reject("socketId and dataB64 are required")
            return
        }
        val conn = sockets[socketId]
        if (conn == null) {
            call.reject("Unknown socket $socketId")
            return
        }
        ioPool.execute {
            try {
                val bytes = Base64.decode(dataB64, Base64.NO_WRAP)
                val out = conn.socket.getOutputStream()
                out.write(bytes)
                out.flush()
                call.resolve()
            } catch (e: Exception) {
                call.reject(e.message ?: "write failed")
            }
        }
    }

    @PluginMethod
    fun close(call: PluginCall) {
        val socketId = call.getString("socketId")
        if (socketId == null) {
            call.reject("socketId is required")
            return
        }
        cleanup(socketId)
        call.resolve()
    }

    /** Report the Wi-Fi gateway / DHCP-server + interface IP for discovery. */
    @PluginMethod
    fun getWifiInfo(call: PluginCall) {
        val result = JSObject()
        try {
            val wifi = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
            @Suppress("DEPRECATION")
            val dhcp: DhcpInfo? = wifi.dhcpInfo
            result.put("gateway", if (dhcp != null) intToIp(dhcp.gateway) else null)
            result.put("ipAddress", if (dhcp != null) intToIp(dhcp.ipAddress) else null)
        } catch (e: Exception) {
            result.put("gateway", null)
            result.put("ipAddress", null)
        }
        call.resolve(result)
    }

    @PluginMethod
    fun getNetworkCapabilities(call: PluginCall) {
        val result = JSObject()
        result.put("isSplitRoutingSupported", true)
        result.put("platform", "android")
        call.resolve(result)
    }

    /** DhcpInfo stores addresses as little-endian ints. */
    private fun intToIp(addr: Int): String? {
        if (addr == 0) return null
        return "${addr and 0xff}.${addr shr 8 and 0xff}.${addr shr 16 and 0xff}.${addr shr 24 and 0xff}"
    }

    private fun cleanup(socketId: String) {
        val conn = sockets.remove(socketId) ?: return
        try {
            conn.socket.close()
        } catch (_: Exception) {
        }
        val cb = conn.networkCallback
        if (cb != null) {
            try {
                connectivityManager().unregisterNetworkCallback(cb)
            } catch (_: Exception) {
            }
        }
    }

    private fun emitClosed(socketId: String) {
        val event = JSObject()
        event.put("socketId", socketId)
        notifyListeners("closed", event)
    }

    private fun emitError(socketId: String, message: String) {
        val event = JSObject()
        event.put("socketId", socketId)
        event.put("message", message)
        notifyListeners("error", event)
    }
}
