// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN multicast presence beacons (matches core/lanbeacon).

package net.quakemesh.android.transport

import org.json.JSONObject

/** Wire format shared with QuakeMeshHub [core/lanbeacon]. */
object LanBeacon {
    const val KIND_HUB = "hub"
    const val KIND_NODE = "node"

    private val PREFIX = byteArrayOf('Q'.code.toByte(), 'M'.code.toByte(), 'L'.code.toByte(), 'B'.code.toByte(), 1)

    data class HubBeacon(
        val nodeId: String,
        val heartbeatPort: Int,
        val ogmPort: Int,
    )

    fun isBeacon(payload: ByteArray): Boolean {
        if (payload.size < PREFIX.size) return false
        return PREFIX.indices.all { payload[it] == PREFIX[it] }
    }

    fun encodeNode(
        nodeId: String,
        lat: Double?,
        lon: Double?,
        accuracyM: Double?,
    ): ByteArray {
        val json = JSONObject()
            .put("v", 1)
            .put("kind", KIND_NODE)
            .put("node_id", nodeId)
        if (lat != null && lon != null) {
            json.put("lat", lat).put("lon", lon)
            if (accuracyM != null) {
                json.put("accuracy_m", accuracyM)
            }
        }
        return PREFIX + json.toString().toByteArray(Charsets.UTF_8)
    }

    fun decodeHub(payload: ByteArray): HubBeacon? {
        if (!isBeacon(payload)) return null
        val json = JSONObject(String(payload, PREFIX.size, payload.size - PREFIX.size, Charsets.UTF_8))
        if (json.optString("kind") != KIND_HUB) return null
        val nodeId = json.optString("node_id")
        if (nodeId.isBlank()) return null
        val heartbeatPort = json.optInt("heartbeat_port", 0)
        val ogmPort = json.optInt("ogm_port", 0)
        if (heartbeatPort <= 0 || ogmPort <= 0) return null
        return HubBeacon(nodeId, heartbeatPort, ogmPort)
    }
}
