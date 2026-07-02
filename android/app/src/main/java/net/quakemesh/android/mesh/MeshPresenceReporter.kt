// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.17 - POST periodic heartbeats to QuakeMeshHub for Monitor visibility.
//   0.0.19 - optional lan_context on heartbeat for infrastructure segments.
//   0.0.25 - Join worker thread on stop for restart-safe lifecycle.

package net.quakemesh.android.mesh

import android.content.Context
import android.util.Log
import net.quakemesh.android.location.LocationReporter
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import kotlin.concurrent.thread

/**
 * Reports this device's mesh node ID (and GPS when available) to a
 * QuakeMeshHub [node heartbeat] endpoint so the node appears in Monitor.
 */
class MeshPresenceReporter(
    private val context: Context,
    private val nodeIdHex: String,
    private val hubBaseUrl: String,
    private val location: () -> LocationReporter.LocationFix?,
) {
    private var running = false
    private var worker: Thread? = null

    fun start(sendImmediately: Boolean = false) {
        if (hubBaseUrl.isBlank()) return
        stop()
        running = true
        worker = thread(name = "MeshPresence") {
            if (sendImmediately) {
                sendHeartbeat()
            }
            while (running) {
                try {
                    Thread.sleep(INTERVAL_MS)
                } catch (_: InterruptedException) {
                    break
                }
                if (!running) break
                sendHeartbeat()
            }
        }
    }

    fun stop() {
        running = false
        worker?.interrupt()
        runCatching { worker?.join(2_000) }
        worker = null
    }

    private fun sendHeartbeat() {
        val base = hubBaseUrl.trimEnd('/')
        val url = URL("$base/v1/heartbeat")
        try {
            val body = JSONObject().put("node_id", nodeIdHex)
            location()?.let { fix ->
                body.put("lat", fix.lat)
                    .put("lon", fix.lon)
                    .put("accuracy_m", fix.accuracyM.toDouble())
            }
            LanContextCollector.collect(context)?.let { lan ->
                val ctx = JSONObject().put("gateway_ip", lan.gatewayIp)
                lan.localIp?.let { ctx.put("local_ip", it) }
                lan.ssid?.let { ctx.put("ssid", it) }
                lan.bssid?.let { ctx.put("bssid", it) }
                body.put("lan_context", ctx)
            }
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            conn.connectTimeout = 10_000
            conn.readTimeout = 10_000
            conn.outputStream.use { it.write(body.toString().toByteArray()) }
            val code = conn.responseCode
            conn.disconnect()
            if (code in 200..299) {
                Log.i(TAG, "heartbeat ok to $base")
            } else {
                Log.w(TAG, "heartbeat HTTP $code to $base")
            }
        } catch (e: Exception) {
            Log.w(TAG, "heartbeat failed: ${e.message}")
        }
    }

    companion object {
        private const val TAG = "MeshPresence"
        private const val INTERVAL_MS = 30_000L
    }
}
