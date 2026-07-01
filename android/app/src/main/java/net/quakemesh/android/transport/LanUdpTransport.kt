// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: connected Wi-Fi LAN multicast discovery + UDP frames.

package net.quakemesh.android.transport

import android.content.Context
import android.net.wifi.WifiManager
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
    private val onFrame: (peerHex: String, frame: ByteArray) -> Unit,
) : Transport {

    override val name: String = "lan-udp"

    private val running = AtomicBoolean(false)
    private var socket: MulticastSocket? = null
    private var multicastLock: WifiManager.MulticastLock? = null
    private var reader: Thread? = null

    override fun start() {
        if (!running.compareAndSet(false, true)) return
        thread(name = "lan-udp-receiver", isDaemon = true) {
            runCatching { runReceiver() }.onFailure {
                running.set(false)
            }
        }
    }

    override fun stop() {
        running.set(false)
        reader?.interrupt()
        socket?.close()
        multicastLock?.release()
        socket = null
        multicastLock = null
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
            val buf = ByteArray(MAX_DATAGRAM)
            while (running.get()) {
                val packet = DatagramPacket(buf, buf.size)
                ms.receive(packet)
                val peer = packet.address.hostAddress ?: continue
                val payload = packet.data.copyOf(packet.length)
                if (payload.isNotEmpty()) {
                    onFrame(peer, payload)
                }
            }
            ms.leaveGroup(group)
        }
    }

    companion object {
        const val MULTICAST_GROUP = "239.255.42.99"
        const val MULTICAST_PORT = 47223
        const val UNICAST_PORT = 47224
        const val MAX_DATAGRAM = 2048
    }
}
