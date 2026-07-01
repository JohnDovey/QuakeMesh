// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: Wi-Fi Direct + Local Only Hotspot skeleton following
//           the Meshrabiya multi-hop pattern (github.com/UstadMobile/Meshrabiya).

package net.quakemesh.android.transport

import android.content.Context
import android.net.wifi.WifiManager
import android.os.Build
import java.util.concurrent.atomic.AtomicBoolean

/**
 * High-bandwidth P2P transport skeleton modelled on Meshrabiya:
 *
 * - Each device runs a **Local Only Hotspot** for downstream peers.
 * - Each device connects as a **Wi-Fi Direct group client** to an upstream node.
 * - Virtual link-local IPs in 169.254.0.0/16 carry routed mesh frames.
 *
 * Phase 4 wires the lifecycle hooks; full Meshrabiya integration lands as
 * the multi-hop path is validated on physical hardware (Phase 4 demo).
 */
class WifiMeshTransport(
    private val context: Context,
    @Suppress("UNUSED_PARAMETER") private val onFrame: (peerHex: String, frame: ByteArray) -> Unit,
) : Transport {

    override val name: String = "wifi-mesh"

    private val running = AtomicBoolean(false)

    override fun start() {
        if (!running.compareAndSet(false, true)) return
        val wifi = context.applicationContext.getSystemService(WifiManager::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            // LocalOnlyHotspot reservation + WifiP2pManager group join — Phase 4 skeleton.
            @Suppress("UNUSED_VARIABLE")
            val unused = wifi
        }
    }

    override fun stop() {
        running.set(false)
    }

    override fun send(peerHex: String, frame: ByteArray) {
        // Virtual IP delivery over the Meshrabiya-style interface — not yet wired.
    }
}
