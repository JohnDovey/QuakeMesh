// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: coordinates mesh node + platform transports.
//   0.0.10 - Phase 8: LocationReporter for GPS sampling.
//   0.0.18 - LAN hub auto-discovery + optional manual heartbeat URL.

package net.quakemesh.android.mesh

import android.content.Context
import net.quakemesh.android.location.LocationReporter
import net.quakemesh.android.transport.BleTransport
import net.quakemesh.android.transport.LanUdpTransport
import net.quakemesh.android.transport.Transport
import net.quakemesh.android.transport.WifiMeshTransport
import java.util.concurrent.CopyOnWriteArrayList

object MeshEngine {
    private val transports = CopyOnWriteArrayList<Transport>()
    private var node: MeshNode? = null
    private var locationReporter: LocationReporter? = null
    private var presenceReporter: MeshPresenceReporter? = null
    private val statusListeners = CopyOnWriteArrayList<(String) -> Unit>()

    var isRunning: Boolean = false
        private set

    fun addStatusListener(listener: (String) -> Unit) {
        statusListeners.add(listener)
    }

    fun removeStatusListener(listener: (String) -> Unit) {
        statusListeners.remove(listener)
    }

    private fun notifyStatus(msg: String) {
        statusListeners.forEach { listener ->
            runCatching { listener(msg) }
        }
    }
    var hubHeartbeatUrl: String = ""
    var hubManualOverride: Boolean = false
    var discoveredHubUrl: String = ""
        private set

    fun prepareStart(manualHubUrl: String) {
        hubManualOverride = manualHubUrl.isNotBlank()
        hubHeartbeatUrl = manualHubUrl
        discoveredHubUrl = ""
    }

    fun start(context: Context) {
        if (node != null) return
        val n = MeshNodeFactory.open(context)
        node = n
        MeshDiscovery.setLocalNodeId(n.nodeId)
        locationReporter = LocationReporter(context.applicationContext).also { it.start() }
        MeshLocalApi.start(n.nodeId)
        if (n is StubMeshNode) {
            n.setOutboundHandler { peer, frame ->
                transports.forEach { it.send(peer, frame) }
            }
        }
        val lan = LanUdpTransport(
            context,
            nodeIdHex = { node?.nodeId },
            location = {
                locationReporter?.latestFix()?.let { Triple(it.lat, it.lon, it.accuracyM) }
            },
            onHubDiscovered = { url -> onHubDiscovered(url) },
        ) { peer, frame -> n.onFrameReceived(peer, frame) }
        val ble = BleTransport(context) { peer, frame -> n.onFrameReceived(peer, frame) }
        val wifi = WifiMeshTransport(context) { peer, frame -> n.onFrameReceived(peer, frame) }
        listOf(lan, ble, wifi).forEach {
            transports.add(it)
            it.start()
        }
        startPresenceReporter()
        val hubNote = when {
            hubHeartbeatUrl.isNotBlank() && hubManualOverride -> " (manual hub URL)"
            hubHeartbeatUrl.isNotBlank() -> " (hub $hubHeartbeatUrl)"
            else -> " (discovering hub on LAN…)"
        }
        isRunning = true
        notifyStatus("Mesh running — node ${n.nodeId.take(12)}…$hubNote")
    }

    fun stop() {
        if (!isRunning && node == null) return
        MeshLocalApi.stop()
        presenceReporter?.stop()
        presenceReporter = null
        locationReporter?.stop()
        locationReporter = null
        transports.forEach { it.stop() }
        transports.clear()
        node?.close()
        node = null
        discoveredHubUrl = ""
        MeshDiscovery.setLocalNodeId(null)
        MeshDiscovery.clear()
        isRunning = false
        notifyStatus("Mesh stopped")
    }

    fun nodeId(): String? = node?.nodeId

    fun latestLocation(): LocationReporter.LocationFix? = locationReporter?.latestFix()

    fun locationSummary(): String? {
        val fix = locationReporter?.latestFix() ?: return null
        return String.format("%.5f, %.5f (±%.0f m)", fix.lat, fix.lon, fix.accuracyM)
    }

    internal fun dispatchOutbound(peerHex: String, frame: ByteArray) {
        transports.forEach { it.send(peerHex, frame) }
    }

    private fun onHubDiscovered(url: String) {
        if (hubManualOverride && hubHeartbeatUrl.isNotBlank()) return
        if (url == discoveredHubUrl && hubHeartbeatUrl == url) return
        discoveredHubUrl = url
        hubHeartbeatUrl = url
        startPresenceReporter()
        val base = "Hub discovered at $url"
        val loc = locationSummary()
        notifyStatus(if (loc != null) "$base\nGPS: $loc" else base)
    }

    private fun startPresenceReporter() {
        val n = node ?: return
        if (hubHeartbeatUrl.isBlank()) return
        presenceReporter?.stop()
        presenceReporter = MeshPresenceReporter(context.applicationContext, n.nodeId, hubHeartbeatUrl) {
            locationReporter?.latestFix()
        }.also { it.start(sendImmediately = true) }
    }
}
