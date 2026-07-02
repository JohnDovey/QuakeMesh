// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN peer registry for the nearby peers drawer.

package net.quakemesh.android.mesh

import java.util.concurrent.ConcurrentHashMap

/** Tracks hubs and nodes heard via LAN multicast beacons. */
object MeshDiscovery {
    enum class Kind { HUB, NODE }

    data class Peer(
        val kind: Kind,
        val address: String,
        val nodeId: String,
        val heartbeatUrl: String?,
        val lat: Double?,
        val lon: Double?,
        val lastSeenMs: Long,
    ) {
        val key: String get() = "${kind.name}:$address:$nodeId"
    }

    private val peers = ConcurrentHashMap<String, Peer>()
    private var localNodeId: String? = null

    var listener: (() -> Unit)? = null

    fun setLocalNodeId(nodeId: String?) {
        localNodeId = nodeId
    }

    fun recordHub(address: String, nodeId: String, heartbeatUrl: String) {
        val now = System.currentTimeMillis()
        peers["${Kind.HUB.name}:$address:$nodeId"] = Peer(
            kind = Kind.HUB,
            address = address,
            nodeId = nodeId,
            heartbeatUrl = heartbeatUrl,
            lat = null,
            lon = null,
            lastSeenMs = now,
        )
        listener?.invoke()
    }

    fun recordNode(address: String, nodeId: String, lat: Double?, lon: Double?) {
        if (nodeId == localNodeId) return
        val now = System.currentTimeMillis()
        peers["${Kind.NODE.name}:$address:$nodeId"] = Peer(
            kind = Kind.NODE,
            address = address,
            nodeId = nodeId,
            heartbeatUrl = null,
            lat = lat,
            lon = lon,
            lastSeenMs = now,
        )
        listener?.invoke()
    }

    fun peers(): List<Peer> = peers.values.sortedWith(
        compareBy<Peer> { it.kind.ordinal }.thenByDescending { it.lastSeenMs },
    )

    fun clear() {
        peers.clear()
        listener?.invoke()
    }
}
