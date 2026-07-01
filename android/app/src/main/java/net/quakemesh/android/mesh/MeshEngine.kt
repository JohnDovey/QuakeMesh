// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: coordinates mesh node + platform transports.
//   0.0.10 - Phase 8: LocationReporter for GPS sampling.

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

  var statusListener: ((String) -> Unit)? = null
  var hubHeartbeatUrl: String = ""

    fun start(context: Context) {
        if (node != null) return
        val n = MeshNodeFactory.open(context)
        node = n
        locationReporter = LocationReporter(context.applicationContext).also { it.start() }
        MeshLocalApi.start(n.nodeId)
        if (n is StubMeshNode) {
            n.setOutboundHandler { peer, frame ->
                transports.forEach { it.send(peer, frame) }
            }
        }
        val lan = LanUdpTransport(context) { peer, frame -> n.onFrameReceived(peer, frame) }
        val ble = BleTransport(context) { peer, frame -> n.onFrameReceived(peer, frame) }
        val wifi = WifiMeshTransport(context) { peer, frame -> n.onFrameReceived(peer, frame) }
        listOf(lan, ble, wifi).forEach {
            transports.add(it)
            it.start()
        }
        if (hubHeartbeatUrl.isNotBlank()) {
            presenceReporter = MeshPresenceReporter(n.nodeId, hubHeartbeatUrl) {
                locationReporter?.latestFix()
            }.also { it.start() }
        }
        statusListener?.invoke("Mesh running — node ${n.nodeId.take(12)}…")
    }

    fun stop() {
        MeshLocalApi.stop()
        presenceReporter?.stop()
        presenceReporter = null
        locationReporter?.stop()
        locationReporter = null
        transports.forEach { it.stop() }
        transports.clear()
        node?.close()
        node = null
        statusListener?.invoke("Mesh stopped")
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
}
