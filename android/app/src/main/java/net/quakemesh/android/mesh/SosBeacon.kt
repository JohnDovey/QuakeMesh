// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.20 - emergency SOS to local mesh API and discovered hub (Monitor).

package net.quakemesh.android.mesh

import android.util.Log
import net.quakemesh.android.location.LocationReporter
import net.quakemesh.sdk.HttpMeshClient
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

/** Publishes an emergency SOS alert on the mesh and to QuakeMeshHub when reachable. */
object SosBeacon {
    private const val TAG = "SosBeacon"
    private const val appId = "net.quakemesh.sosbeacon"
    private const val topic = "sos"
    private const val defaultText = "SOS — need assistance"

    fun send(
        message: String,
        location: LocationReporter.LocationFix?,
        onLine: (String) -> Unit,
    ): Boolean {
        val nodeHex = MeshEngine.nodeId()
        if (nodeHex.isNullOrBlank()) {
            onLine("SOS failed: mesh node not running")
            return false
        }
        val text = message.ifBlank { defaultText }
        val payload = buildPayload(text, nodeHex, location)
        var ok = false
        ok = publishLocal(payload, onLine) || ok
        val hub = MeshEngine.hubHeartbeatUrl.trim()
        if (hub.isNotBlank()) {
            ok = publishHub(hub, payload, onLine) || ok
        } else {
            onLine("hub not discovered — alert is on-device only (Monitor needs same Wi‑Fi hub)")
        }
        return ok
    }

    private fun buildPayload(
        text: String,
        nodeHex: String,
        location: LocationReporter.LocationFix?,
    ): JSONObject {
        val body = JSONObject()
            .put("text", text)
            .put("node_id", nodeHex)
            .put("sent_at", System.currentTimeMillis())
        if (location != null) {
            body.put("lat", location.lat)
                .put("lon", location.lon)
                .put("accuracy_m", location.accuracyM.toDouble())
        }
        return body
    }

    private fun publishLocal(payload: JSONObject, onLine: (String) -> Unit): Boolean {
        return try {
            val client = HttpMeshClient("http://127.0.0.1:${MeshLocalApi.DEFAULT_PORT}")
            val sess = client.register(appId, "SOS Beacon", "0.1.0", listOf("sos", "location"))
            client.publish(sess, topic, payload.toString().toByteArray())
            onLine("SOS published on local mesh (topic $topic)")
            true
        } catch (e: Exception) {
            onLine("local mesh publish failed: ${e.message}")
            false
        }
    }

    private fun publishHub(hubBase: String, payload: JSONObject, onLine: (String) -> Unit): Boolean {
        val base = hubBase.trimEnd('/')
        return try {
            val conn = URL("$base/v1/sos").openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            conn.connectTimeout = 10_000
            conn.readTimeout = 10_000
            conn.outputStream.use { it.write(payload.toString().toByteArray()) }
            val code = conn.responseCode
            conn.disconnect()
            if (code in 200..299) {
                onLine("SOS sent to hub at $base (Monitor SOS Alerts)")
                Log.i(TAG, "hub SOS ok $base")
                true
            } else {
                onLine("hub SOS HTTP $code")
                false
            }
        } catch (e: Exception) {
            onLine("hub SOS failed: ${e.message}")
            false
        }
    }
}
