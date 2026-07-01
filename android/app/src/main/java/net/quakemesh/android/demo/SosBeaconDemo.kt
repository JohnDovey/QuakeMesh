// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Phase 13: publish an SOS alert via HttpMeshClient.

package net.quakemesh.android.demo

import net.quakemesh.android.location.LocationReporter
import net.quakemesh.android.mesh.MeshLocalApi
import net.quakemesh.sdk.HttpMeshClient
import org.json.JSONObject

object SosBeaconDemo {

    private const val appId = "net.quakemesh.sosbeacon"
    private const val topic = "sos"

    fun send(text: String, location: LocationReporter.LocationFix?, onLine: (String) -> Unit): Boolean {
        return try {
            val client = HttpMeshClient("http://127.0.0.1:${MeshLocalApi.DEFAULT_PORT}")
            val sess = client.register(appId, "SOS Beacon", "0.1.0", listOf("sos", "location"))
            val nodeHex = sess.nodeId.joinToString("") { "%02x".format(it) }
            onLine("registered node=${nodeHex.take(16)}…")

            val body = JSONObject()
                .put("text", text)
                .put("node_id", nodeHex)
                .put("sent_at", System.currentTimeMillis())
            if (location != null) {
                body.put("lat", location.lat)
                body.put("lon", location.lon)
                body.put("accuracy_m", location.accuracyM.toDouble())
                onLine("location ${location.lat}, ${location.lon} (±${location.accuracyM.toInt()} m)")
            } else {
                onLine("no GPS fix yet — sending without coordinates")
            }

            client.publish(sess, topic, body.toString().toByteArray())
            onLine("SOS published to topic $topic")
            true
        } catch (e: Exception) {
            onLine("SOS failed: ${e.message}")
            false
        }
    }
}
