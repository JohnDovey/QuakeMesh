// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Loopback mesh-sdk transport for third-party and standalone mesh apps.

package net.quakemesh.meshapps

import net.quakemesh.sdk.HttpMeshClient
import java.net.InetSocketAddress
import java.net.Socket

/** Talks to QuakeMesh's loopback mesh-sdk daemon on 127.0.0.1:18084. */
object MeshTransport {
    const val DEFAULT_PORT = 18084

    /** Override in QuakeMesh host app to use MeshEngine.isRunning instead of a socket probe. */
    var availabilityCheck: () -> Boolean = { isLoopbackReachable() }

    fun loopbackUrl(port: Int = DEFAULT_PORT): String = "http://127.0.0.1:$port"

    fun newClient(port: Int = DEFAULT_PORT): HttpMeshClient = HttpMeshClient(loopbackUrl(port))

    fun meshAvailable(): Boolean = availabilityCheck()

    fun bytesToHex(bytes: ByteArray): String =
        bytes.joinToString("") { "%02x".format(it) }

    fun hexToBytes(hex: String): ByteArray {
        val clean = hex.trim().removePrefix("0x")
        require(clean.length % 2 == 0) { "invalid hex node id" }
        return clean.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
    }

    fun shortId(hex: String): String =
        if (hex.length <= 12) hex else hex.take(8) + "…"

    private fun isLoopbackReachable(port: Int = DEFAULT_PORT): Boolean {
        return try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress("127.0.0.1", port), 1_500)
                true
            }
        } catch (_: Exception) {
            false
        }
    }
}
