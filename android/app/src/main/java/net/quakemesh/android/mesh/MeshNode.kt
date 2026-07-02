// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: abstract mesh node facade for Go AAR or stub.

package net.quakemesh.android.mesh

import android.content.Context
import java.io.File

/** Kotlin-facing mesh node; backed by gomobile [mobile.Node] when AAR is present. */
interface MeshNode {
    val nodeId: String
    fun onFrameReceived(peerHex: String, frame: ByteArray)
    fun emitFrame(peerHex: String, frame: ByteArray)
    fun close()
}

object MeshNodeFactory {
    fun open(context: Context): MeshNode {
        val files = context.filesDir
        val identity = File(files, "quakemesh.identity").absolutePath
        val db = File(files, "quakemesh.db").absolutePath
        if (GoMeshNode.isAvailable) {
            runCatching { return GoMeshNode(identity, db) }
                .onFailure { android.util.Log.w("MeshNodeFactory", "Go core failed, using stub: ${it.message}") }
        }
        return StubMeshNode.open(context, identity, db)
    }
}
