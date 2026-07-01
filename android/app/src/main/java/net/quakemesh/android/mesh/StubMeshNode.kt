// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: stub mesh node for builds without meshcore.aar.

package net.quakemesh.android.mesh

import android.content.Context
import java.security.MessageDigest
import java.util.UUID
import kotlin.random.Random

/**
 * Stand-in when [meshcore.aar] has not been built yet (requires Android NDK +
 * [gomobile-bind.sh]). Generates a stable pseudo NodeID persisted in app
 * storage so transport shims can be exercised on emulators.
 */
class StubMeshNode private constructor(
    override val nodeId: String,
) : MeshNode {

    private var outbound: ((String, ByteArray) -> Unit)? = null

    fun setOutboundHandler(handler: (peerHex: String, frame: ByteArray) -> Unit) {
        outbound = handler
    }

    override fun onFrameReceived(peerHex: String, frame: ByteArray) {
        // Phase 4: accept frames; routing lands in Phase 5.
    }

    override fun emitFrame(peerHex: String, frame: ByteArray) {
        outbound?.invoke(peerHex, frame)
    }

    override fun close() {}

    companion object {
        fun open(context: Context, identityPath: String, @Suppress("UNUSED_PARAMETER") dbPath: String): StubMeshNode {
            val prefs = context.getSharedPreferences("quakemesh_stub", Context.MODE_PRIVATE)
            var id = prefs.getString("node_id", null)
            if (id == null) {
                val seed = UUID.randomUUID().toString().toByteArray()
                val digest = MessageDigest.getInstance("SHA-256").digest(seed)
                id = digest.joinToString("") { "%02x".format(it) }
                prefs.edit().putString("node_id", id).apply()
            }
            return StubMeshNode(id)
        }
    }
}
