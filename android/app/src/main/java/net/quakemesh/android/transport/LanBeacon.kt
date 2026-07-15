// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN multicast presence beacons (matches core/lanbeacon).
//   0.0.19 - optional lan_context on node beacons.

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

    data class LanContext(
        val gatewayIp: String,
        val localIp: String? = null,
        val ssid: String? = null,
        val bssid: String? = null,
    )

    fun encodeNode(
        nodeId: String,
        handle: String? = null,
        homeLat: Double? = null,
        homeLon: Double? = null,
        lat: Double?,
        lon: Double?,
        accuracyM: Double?,
        lan: LanContext? = null,
    ): ByteArray {
        val json = JSONObject()
            .put("v", 1)
            .put("kind", KIND_NODE)
            .put("node_id", nodeId)
        if (!handle.isNullOrBlank()) {
            json.put("handle", handle.trim())
        }
        if (homeLat != null && homeLon != null) {
            json.put("home_lat", homeLat).put("home_lon", homeLon)
        }
        if (lat != null && lon != null) {
            json.put("lat", lat).put("lon", lon)
            if (accuracyM != null) {
                json.put("accuracy_m", accuracyM)
            }
        }
        lan?.let { putLanContext(json, it) }
        return PREFIX + json.toString().toByteArray(Charsets.UTF_8)
    }

    private fun putLanContext(json: JSONObject, lan: LanContext) {
        val ctx = JSONObject().put("gateway_ip", lan.gatewayIp)
        lan.localIp?.let { ctx.put("local_ip", it) }
        lan.ssid?.let { ctx.put("ssid", it) }
        lan.bssid?.let { ctx.put("bssid", it) }
        json.put("lan_context", ctx)
    }

    fun lanContextFromJson(obj: JSONObject?): LanContext? {
        if (obj == null) return null
        val gateway = obj.optString("gateway_ip")
        if (gateway.isBlank()) return null
        return LanContext(
            gatewayIp = gateway,
            localIp = obj.optString("local_ip").takeIf { it.isNotBlank() },
            ssid = obj.optString("ssid").takeIf { it.isNotBlank() },
            bssid = obj.optString("bssid").takeIf { it.isNotBlank() },
        )
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

    data class NodeBeacon(
        val nodeId: String,
        val handle: String?,
        val homeLat: Double?,
        val homeLon: Double?,
        val lat: Double?,
        val lon: Double?,
    )

    fun decodeNode(payload: ByteArray): NodeBeacon? {
        if (!isBeacon(payload)) return null
        val json = JSONObject(String(payload, PREFIX.size, payload.size - PREFIX.size, Charsets.UTF_8))
        if (json.optString("kind") != KIND_NODE) return null
        val nodeId = json.optString("node_id")
        if (nodeId.isBlank()) return null
        val handle = json.optString("handle").takeIf { it.isNotBlank() }
        val homeLat = if (json.has("home_lat")) json.optDouble("home_lat") else null
        val homeLon = if (json.has("home_lon")) json.optDouble("home_lon") else null
        val lat = if (json.has("lat")) json.optDouble("lat") else null
        val lon = if (json.has("lon")) json.optDouble("lon") else null
        return NodeBeacon(nodeId, handle, homeLat, homeLon, lat, lon)
    }
}
