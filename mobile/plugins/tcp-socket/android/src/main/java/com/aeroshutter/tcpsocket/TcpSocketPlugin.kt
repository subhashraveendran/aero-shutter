package com.aeroshutter.tcpsocket

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.DhcpInfo
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.wifi.WifiManager
import android.net.wifi.WifiNetworkSpecifier
import android.os.Build
import android.os.PatternMatcher
import android.util.Base64
import com.getcapacitor.JSObject
import com.getcapacitor.JSArray
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import com.getcapacitor.annotation.Permission
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference

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
 *
 * Race + fallback (why it now connects reliably)
 * ----------------------------------------------
 * The Wi-Fi-bound path only works if a bound Wi-Fi network is actually granted.
 * On some phones the request is slow, denied, or the bound socket can't reach
 * the camera even though a plain default socket can (e.g. the phone has NO SIM,
 * so the DEFAULT network already IS the camera Wi-Fi). Previously, if acquiring
 * or binding failed we gave up without ever trying a plain socket.
 *
 * Now, when bindWifi is requested we RACE two attempts and use whichever
 * connects first:
 *   1. a Wi-Fi-bound socket (short bounded wait to acquire the Wi-Fi network),
 *   2. a plain default-network socket.
 * Whichever wins is kept; the loser is closed and its Wi-Fi request released.
 * This means it works whether or not cellular is present, and a stall on one
 * path never blocks the other.
 */
@CapacitorPlugin(
    name = "TcpSocket",
    permissions = [
        Permission(
            alias = "location",
            strings = [Manifest.permission.ACCESS_FINE_LOCATION],
        ),
    ],
)
class TcpSocketPlugin : Plugin() {

    private data class Conn(
        val socket: Socket,
        val networkCallback: ConnectivityManager.NetworkCallback?,
    )

    private val sockets = ConcurrentHashMap<String, Conn>()
    private val idCounter = AtomicInteger(0)
    private val ioPool = Executors.newCachedThreadPool()

    /**
     * Long-lived callback for an app-requested Wi-Fi network joined via
     * joinWifi(). Kept so leaveWifi() can unregister it and unbind the process.
     * Distinct from the per-socket bind-race callbacks in connect().
     */
    private var joinCallback: ConnectivityManager.NetworkCallback? = null
    private var joinedSsid: String? = null

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
            try {
                val outcome = if (bindWifi) {
                    raceWifiAndDefault(host, port, timeoutMs)
                } else {
                    val socket = Socket()
                    socket.tcpNoDelay = true
                    socket.connect(InetSocketAddress(host, port), timeoutMs)
                    ConnectOutcome(socket, null, "default")
                }

                sockets[socketId] = Conn(outcome.socket, outcome.networkCallback)

                val result = JSObject()
                result.put("socketId", socketId)
                result.put("networkBinding", outcome.binding)
                call.resolve(result)

                readLoop(socketId, outcome.socket)
            } catch (e: Exception) {
                call.reject(e.message ?: "connect failed")
            }
        }
    }

    private data class ConnectOutcome(
        val socket: Socket,
        val networkCallback: ConnectivityManager.NetworkCallback?,
        val binding: String,
    )

    /**
     * Race a Wi-Fi-bound socket against a plain default-network socket and return
     * whichever connects first; close/release the loser. This is the core of the
     * "it actually connects" fix: it works whether or not cellular is present,
     * and a stall or denial on the Wi-Fi-bind path never blocks the default path.
     *
     * If BOTH attempts fail, the last error is rethrown so the caller can reject.
     */
    private fun raceWifiAndDefault(host: String, port: Int, timeoutMs: Int): ConnectOutcome {
        val cm = connectivityManager()
        // Winner handoff: first successful ConnectOutcome wins. `claimed` ensures
        // only one outcome is kept; any later winner is closed instead.
        val winnerRef = AtomicReference<ConnectOutcome?>(null)
        val errorRef = AtomicReference<Exception?>(null)
        val claimed = java.util.concurrent.atomic.AtomicBoolean(false)
        val done = CountDownLatch(2)

        // Bound the Wi-Fi acquire to a short window so it can never hang the whole
        // connect: at most ~3.5s (and never more than the overall timeout).
        val wifiAcquireMs = minOf(timeoutMs, 3500)

        // Track the callback so we can unregister it if the Wi-Fi socket loses.
        val callbackRef = AtomicReference<ConnectivityManager.NetworkCallback?>(null)

        fun tryClaim(outcome: ConnectOutcome) {
            if (claimed.compareAndSet(false, true)) {
                winnerRef.set(outcome)
            } else {
                // Someone else already won: discard this socket + release Wi-Fi.
                try { outcome.socket.close() } catch (_: Exception) {}
                outcome.networkCallback?.let {
                    try { cm.unregisterNetworkCallback(it) } catch (_: Exception) {}
                }
            }
        }

        // Path 1: Wi-Fi-bound socket.
        ioPool.execute {
            try {
                val acquired = acquireWifiNetwork(wifiAcquireMs)
                if (acquired != null) {
                    callbackRef.set(acquired.second)
                    val socket = acquired.first.socketFactory.createSocket() as Socket
                    socket.tcpNoDelay = true
                    try {
                        socket.connect(InetSocketAddress(host, port), timeoutMs)
                        tryClaim(ConnectOutcome(socket, acquired.second, "wifi-bound"))
                    } catch (e: Exception) {
                        try { socket.close() } catch (_: Exception) {}
                        try { cm.unregisterNetworkCallback(acquired.second) } catch (_: Exception) {}
                        errorRef.set(e)
                    }
                }
            } catch (e: Exception) {
                errorRef.set(e)
            } finally {
                done.countDown()
            }
        }

        // Path 2: plain default-network socket (covers no-SIM / bind-denied cases).
        ioPool.execute {
            try {
                val socket = Socket()
                socket.tcpNoDelay = true
                socket.connect(InetSocketAddress(host, port), timeoutMs)
                tryClaim(ConnectOutcome(socket, null, "default"))
            } catch (e: Exception) {
                errorRef.set(e)
            } finally {
                done.countDown()
            }
        }

        // Wait until a winner appears or both paths finish. Poll the winner so we
        // return as soon as the first success lands rather than waiting for both.
        val deadline = System.currentTimeMillis() + timeoutMs + 500L
        while (System.currentTimeMillis() < deadline) {
            if (winnerRef.get() != null) break
            if (done.await(50, TimeUnit.MILLISECONDS)) break
        }

        val winner = winnerRef.get()
        if (winner != null) return winner

        throw errorRef.get() ?: java.net.SocketTimeoutException("connect timed out")
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
        // AtomicReference gives a safe cross-thread handoff from the callback
        // thread to the waiting caller (Kotlin forbids @Volatile on locals).
        val networkRef = AtomicReference<Network?>(null)
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(available: Network) {
                networkRef.set(available)
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

        val net = networkRef.get()
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

    // ===================================================================
    //  In-app Wi-Fi joining
    // ===================================================================

    /**
     * Join a Wi-Fi network from inside the app.
     *
     * API 29+ (primary path): build a NetworkRequest with TRANSPORT_WIFI and a
     * WifiNetworkSpecifier for the SSID (exact via setSsid, or PREFIX match via
     * setSsidPattern for the "Nikon_WU2_" case). If a password is supplied we
     * set a WPA2 passphrase; otherwise the network is treated as open (typical
     * for a Nikon camera AP). ConnectivityManager.requestNetwork shows the
     * system's "connect to this Wi-Fi?" dialog; on onAvailable() we
     * bindProcessToNetwork(network) so all subsequent camera sockets route to
     * the AP. The callback is retained so leaveWifi() can tear it down.
     *
     * API < 29: no WifiNetworkSpecifier — return joined=false so the UI tells
     * the user to join manually via system settings.
     */
    @PluginMethod
    fun joinWifi(call: PluginCall) {
        val ssid = call.getString("ssid")
        val password = call.getString("password")
        val ssidPrefix = call.getString("ssidPrefix")

        if (ssid.isNullOrEmpty() && ssidPrefix.isNullOrEmpty()) {
            call.reject("ssid or ssidPrefix is required")
            return
        }

        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            // Pre-Android 10: WifiNetworkSpecifier is unavailable. We could use
            // the deprecated WifiManager.addNetwork/enableNetwork path, but it
            // is unreliable and largely no-ops on modern OEM builds, so we
            // honestly report failure and let the UI fall back to Settings.
            val result = JSObject()
            result.put("joined", false)
            result.put("ssid", ssid ?: (ssidPrefix ?: ""))
            result.put("bound", false)
            call.resolve(result)
            return
        }

        // If only a prefix is given, try to resolve it to a concrete SSID from a
        // scan (needs location perms). If we can't, we still request via a
        // prefix PatternMatcher so the OS can offer matching APs.
        val resolvedSsid: String? = ssid ?: ssidPrefix?.let { firstScanMatch(it) }

        val specifierBuilder = WifiNetworkSpecifier.Builder()
        if (resolvedSsid != null) {
            specifierBuilder.setSsid(resolvedSsid)
        } else if (ssidPrefix != null) {
            specifierBuilder.setSsidPattern(
                PatternMatcher(ssidPrefix, PatternMatcher.PATTERN_PREFIX),
            )
        }
        if (!password.isNullOrEmpty()) {
            specifierBuilder.setWpa2Passphrase(password)
        }

        val cm = connectivityManager()
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            // The camera AP has no internet; do NOT require NET_CAPABILITY_INTERNET.
            .setNetworkSpecifier(specifierBuilder.build())
            .build()

        // Release any previous join before requesting a new one.
        releaseJoin(cm)

        val settled = java.util.concurrent.atomic.AtomicBoolean(false)
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                val bound = try {
                    cm.bindProcessToNetwork(network)
                } catch (_: Exception) {
                    false
                }
                joinedSsid = resolvedSsid ?: ssidPrefix
                if (settled.compareAndSet(false, true)) {
                    val result = JSObject()
                    result.put("joined", true)
                    result.put("ssid", joinedSsid ?: "")
                    result.put("bound", bound)
                    call.resolve(result)
                }
            }

            override fun onUnavailable() {
                if (settled.compareAndSet(false, true)) {
                    val result = JSObject()
                    result.put("joined", false)
                    result.put("ssid", resolvedSsid ?: (ssidPrefix ?: ""))
                    result.put("bound", false)
                    call.resolve(result)
                }
            }
        }

        joinCallback = callback
        try {
            // Give the user generous time to accept the system Wi-Fi dialog.
            cm.requestNetwork(request, callback, 30_000)
        } catch (e: Exception) {
            joinCallback = null
            call.reject(e.message ?: "joinWifi failed")
        }
    }

    /** Best-effort readout of the currently-joined SSID. */
    @PluginMethod
    fun currentWifi(call: PluginCall) {
        val result = JSObject()
        result.put("ssid", currentSsid())
        call.resolve(result)
    }

    /** Best-effort scan of visible SSIDs (empty if location denied/off). */
    @PluginMethod
    fun scanWifi(call: PluginCall) {
        val networks = JSArray()
        if (hasLocationPermission()) {
            try {
                val wifi = context.applicationContext
                    .getSystemService(Context.WIFI_SERVICE) as WifiManager
                @Suppress("DEPRECATION")
                val results = wifi.scanResults ?: emptyList()
                val seen = HashSet<String>()
                for (r in results) {
                    val ss = r.SSID ?: continue
                    if (ss.isEmpty() || !seen.add(ss)) continue
                    val obj = JSObject()
                    obj.put("ssid", ss)
                    networks.put(obj)
                }
            } catch (_: Exception) {
                // Degrade to empty list.
            }
        }
        val result = JSObject()
        result.put("networks", networks)
        call.resolve(result)
    }

    /** Release any app-requested Wi-Fi binding from joinWifi(). */
    @PluginMethod
    fun leaveWifi(call: PluginCall) {
        releaseJoin(connectivityManager())
        call.resolve()
    }

    private fun releaseJoin(cm: ConnectivityManager) {
        val cb = joinCallback
        joinCallback = null
        joinedSsid = null
        try {
            cm.bindProcessToNetwork(null)
        } catch (_: Exception) {
        }
        if (cb != null) {
            try {
                cm.unregisterNetworkCallback(cb)
            } catch (_: Exception) {
            }
        }
    }

    /** Resolve the first scan SSID starting with [prefix], or null. */
    private fun firstScanMatch(prefix: String): String? {
        if (!hasLocationPermission()) return null
        return try {
            val wifi = context.applicationContext
                .getSystemService(Context.WIFI_SERVICE) as WifiManager
            @Suppress("DEPRECATION")
            wifi.scanResults
                ?.mapNotNull { it.SSID }
                ?.firstOrNull { it.startsWith(prefix) }
        } catch (_: Exception) {
            null
        }
    }

    /**
     * Read the currently-connected SSID. On Android 10+ this requires
     * ACCESS_FINE_LOCATION and location services on; otherwise it is redacted
     * ("<unknown ssid>") and we return null.
     */
    private fun currentSsid(): String? {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q && !hasLocationPermission()) {
            return null
        }
        return try {
            val wifi = context.applicationContext
                .getSystemService(Context.WIFI_SERVICE) as WifiManager
            @Suppress("DEPRECATION")
            val raw = wifi.connectionInfo?.ssid ?: return null
            // WifiInfo wraps the SSID in quotes; strip them. Redacted values
            // ("<unknown ssid>") and empty strings collapse to null.
            val clean = raw.trim('"')
            if (clean.isEmpty() || clean == "<unknown ssid>" || clean == "0x") null else clean
        } catch (_: Exception) {
            null
        }
    }

    private fun hasLocationPermission(): Boolean =
        context.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED

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
