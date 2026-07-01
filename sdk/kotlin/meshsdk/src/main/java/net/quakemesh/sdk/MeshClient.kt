// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

package net.quakemesh.sdk

/**
 * Wraps the local IPC API (AIDL-backed bound service or loopback gRPC on
 * Android) so third-party apps can use the mesh purely as a transport.
 * See "Application SDK and Transport-as-a-Service" in /plan.md.
 *
 * Not yet implemented (Phase 10). Mirrors [/sdk/go's Client] interface.
 */
data class Session(
    val appId: String,
    val appName: String,
    val appVersion: String,
)

interface MeshClient {
    fun register(appId: String, appName: String, appVersion: String, capabilities: List<String>): Session
    fun send(session: Session, destNodeId: ByteArray, payload: ByteArray)
    fun receive(session: Session): Sequence<ByteArray>
    fun publish(session: Session, topic: String, payload: ByteArray)
    fun subscribe(session: Session, topic: String): Sequence<ByteArray>
    fun discoverPeers(appId: String, versionConstraint: String?): List<ByteArray>
}
