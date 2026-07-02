// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: connected Wi-Fi LAN multicast discovery + UDP frames.
//   0.0.18 - LAN presence beacons for hub auto-discovery.
//   0.0.19 - lan_context on node LAN beacons.
//   0.0.25 - Join receiver threads on stop for restart-safe lifecycle.

package net.quakemesh.android.transport

import android.content.Context
import android.net.wifi.WifiManager
import android.util.Log
import net.quakemesh.android.mesh.LanContextCollector
import java.net.DatagramPacket
import java.net.InetAddress
import java.net.MulticastSocket
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

/**
 * Uses the LAN the device is already joined to (see "Local Wi-Fi / LAN" in
 * /plan.md): multicast UDP beacons for discovery and unicast UDP for frames.
 */
class LanUdpTransport(
    private val context: Context,
    private val nodeIdHex: () -> String?,
    private val location: () -> Triple<Double, Double, Float>?,
    private val onHubDiscovered: (String) -> Unit,
    private val onFrame: (peerHex: String, frame: ByteArray) -> Unit,
) : Transport {

    override val name: String = "lan-udp"

    private val lifecycleLock = Any()
    private val running = AtomicBoolean(false)
    private var socket: MulticastSocket? = null
    private var multicastLock: WifiManager.MulticastLock? = null
    private var reader: Thread? = null
    private var beaconSender: Thread? = null

    override fun start() {
        synchronized(lifecycleLock) {
            stopLocked()
            running.set(true)
            reader = thread(name = "lan-udp-receiver", isDaemon = true) {
                try {
                    runReceiver()
                } catch (_: InterruptedException) {
                } catch (e: Exception) {
                    if (running.get()) {
                        Log.w(TAG, "receiver stopped: ${e.message}")
                    }
                } finally {
                    running.set(false)
                }
            }
        }
    }

    override fun stop() {
        synchronized(lifecycleLock) {
            stopLocked()
        }
    }

    private fun stopLocked() {
        running.set(false)
        socket?.close()
        reader?.interrupt()
        beaconSender?.interrupt()
        runCatching { reader?.join(2_000) }
        runCatching { beaconSender?.join(1_000) }
        runCatching {
            multicastLock?.let { lock ->
                if (lock.isHeld) lock.release()
            }
        }
        socket = null
        multicastLock = null
        reader = null
        beaconSender = null
    }

    override fun send(peerHex: String, frame: ByteArray) {
        val sock = socket ?: return
        val addr = InetAddress.getByName(peerHex)
        val packet = DatagramPacket(frame, frame.size, addr, UNICAST_PORT)
        runCatching { sock.send(packet) }
    }

    private fun runReceiver() {
        val wifi = context.applicationContext.getSystemService(WifiManager::class.java)
        multicastLock = wifi.createMulticastLock("quakemesh-lan").apply {
            setReferenceCounted(true)
            acquire()
        }
        val group = InetAddress.getByName(MULTICAST_GROUP)
        MulticastSocket(MULTICAST_PORT).use { ms ->
            socket = ms
            ms.joinGroup(group)
            beaconSender = thread(name = "lan-udp-beacon", isDaemon = true) {
                runCatching { runBeaconSender(group) }
            }
            val buf = ByteArray(MAX_DATAGRAM)
            while (running.get()) {
                val packet = DatagramPacket(buf, buf.size)
                ms.receive(packet)
                val peer = packet.address.hostAddress ?: continue
                val payload = packet.data.copyOf(packet.length)
                if (payload.isEmpty()) continue
                if (LanBeacon.isBeacon(payload)) {
                    LanBeacon.decodeHub(payload)?.let { hub ->
                        val url = "http://$peer:${hub.heartbeatPort}"
                        net.quakemesh.android.mesh.MeshDiscovery.recordHub(peer, hub.nodeId, url)
                        onHubDiscovered(url)
                    } ?: LanBeacon.decodeNode(payload)?.let { node ->
                        net.quakemesh.android.mesh.MeshDiscovery.recordNode(
                            peer, node.nodeId, node.lat, node.lon,
                        )
                    }
                } else {
                    onFrame(peer, payload)
                }
            }
            ms.leaveGroup(group)
        }
    }

    private fun runBeaconSender(group: InetAddress) {
        while (running.get()) {
            val id = nodeIdHex()
            if (id != null) {
                val loc = location()
                val lan = LanContextCollector.collect(context)?.let {
                    LanBeacon.LanContext(it.gatewayIp, it.localIp, it.ssid, it.bssid)
                }
                val payload = LanBeacon.encodeNode(
                    id,
                    loc?.first,
                    loc?.second,
                    loc?.third?.toDouble(),
                    lan,
                )
                val sock = socket
                if (sock != null) {
                    val packet = DatagramPacket(payload, payload.size, group, MULTICAST_PORT)
                    runCatching { sock.send(packet) }
                }
            }
            Thread.sleep(BEACON_INTERVAL_MS)
        }
    }

    companion object {
        private const val TAG = "LanUdpTransport"
        const val MULTICAST_GROUP = "239.255.42.99"
        const val MULTICAST_PORT = 47223
        const val UNICAST_PORT = 47224
        const val MAX_DATAGRAM = 2048
        private const val BEACON_INTERVAL_MS = 5_000L
    }
}
