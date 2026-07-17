package com.aeroshutter.tcpsocket

import android.util.Base64
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger

/**
 * Raw TCP socket bridge for PTP/IP camera connections.
 *
 * Each connection runs on a dedicated background thread that streams 8 KiB
 * reads and emits them as `data` events (base64). `closed` and `error` events
 * signal lifecycle changes. Writes are dispatched to the socket's output stream.
 */
@CapacitorPlugin(name = "TcpSocket")
class TcpSocketPlugin : Plugin() {

    private data class Conn(val socket: Socket)

    private val sockets = ConcurrentHashMap<String, Conn>()
    private val idCounter = AtomicInteger(0)
    private val ioPool = Executors.newCachedThreadPool()

    @PluginMethod
    fun connect(call: PluginCall) {
        val host = call.getString("host")
        val port = call.getInt("port")
        val timeoutMs = call.getInt("timeoutMs", 8000)!!
        if (host == null || port == null) {
            call.reject("host and port are required")
            return
        }
        val socketId = "sock-${idCounter.incrementAndGet()}"

        ioPool.execute {
            try {
                val socket = Socket()
                socket.tcpNoDelay = true
                socket.connect(InetSocketAddress(host, port), timeoutMs)
                sockets[socketId] = Conn(socket)

                val result = JSObject()
                result.put("socketId", socketId)
                call.resolve(result)

                readLoop(socketId, socket)
            } catch (e: Exception) {
                call.reject(e.message ?: "connect failed")
            }
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

    private fun cleanup(socketId: String) {
        val conn = sockets.remove(socketId) ?: return
        try {
            conn.socket.close()
        } catch (_: Exception) {
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
