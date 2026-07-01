// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: coordinates mesh node + platform transports.

package net.quakemesh.android.mesh

import android.content.Context
import net.quakemesh.android.transport.BleTransport
import net.quakemesh.android.transport.LanUdpTransport
import net.quakemesh.android.transport.Transport
import net.quakemesh.android.transport.WifiMeshTransport
import java.util.concurrent.CopyOnWriteArrayList

object MeshEngine {
    private val transports = CopyOnWriteArrayList<Transport>()
    private var node: MeshNode? = null

  var statusListener: ((String) -> Unit)? = null

    fun start(context: Context) {
        if (node != null) return
        val n = MeshNodeFactory.open(context)
        node = n
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
        statusListener?.invoke("Mesh running — node ${n.nodeId.take(12)}…")
    }

    fun stop() {
        transports.forEach { it.stop() }
        transports.clear()
        node?.close()
        node = null
        statusListener?.invoke("Mesh stopped")
    }

    fun nodeId(): String? = node?.nodeId

    internal fun dispatchOutbound(peerHex: String, frame: ByteArray) {
        transports.forEach { it.send(peerHex, frame) }
    }
}
