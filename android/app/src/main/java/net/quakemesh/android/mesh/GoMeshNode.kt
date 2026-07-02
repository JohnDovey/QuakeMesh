// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: gomobile mesh node wrapper (compiled when meshcore.aar
//           is on the classpath).

package net.quakemesh.android.mesh

/**
 * Bridges Kotlin transports to the gomobile-generated [mobile.Node].
 *
 * Build the AAR first:
 *
 *     cd android && ./gomobile-bind.sh
 *
 * The generated types live in the `mobile` Java package (`mobile.Node`,
 * `mobile.FrameSink`).
 */
class GoMeshNode private constructor(
    private val node: Any,
    private val closer: () -> Unit,
) : MeshNode {

    override val nodeId: String
        get() = invokeString(node, "nodeID")

    override fun onFrameReceived(peerHex: String, frame: ByteArray) {
        invoke(node, "onFrameReceived", arrayOf(String::class.java, ByteArray::class.java), arrayOf(peerHex, frame))
    }

    override fun emitFrame(peerHex: String, frame: ByteArray) {
        invoke(node, "emitFrame", arrayOf(String::class.java, ByteArray::class.java), arrayOf(peerHex, frame))
    }

    override fun close() {
        closer()
    }

    companion object {
        val isAvailable: Boolean = runCatching {
            Class.forName("mobile.Node")
        }.isSuccess

        operator fun invoke(identityPath: String, dbPath: String): GoMeshNode {
            val nodeClass = Class.forName("mobile.Node")
            val ctor = nodeClass.getDeclaredConstructor(String::class.java, String::class.java)
            val node = ctor.newInstance(identityPath, dbPath)
            val sinkClass = Class.forName("mobile.FrameSink")
            val proxy = java.lang.reflect.Proxy.newProxyInstance(
                sinkClass.classLoader,
                arrayOf(sinkClass),
            ) { _, method, args ->
                if (method.name == "sendFrame" && args != null) {
                    val peer = args[0] as String
                    val frame = args[1] as ByteArray
                    MeshEngine.dispatchOutbound(peer, frame)
                }
                null
            }
            nodeClass.getDeclaredMethod("setFrameSink", sinkClass).invoke(node, proxy)
            val closer: () -> Unit = {
                nodeClass.getDeclaredMethod("close").invoke(node)
                Unit
            }
            return GoMeshNode(node, closer)
        }
    }

    private fun invokeString(target: Any, method: String): String {
        return target.javaClass.getDeclaredMethod(method).invoke(target) as String
    }

    private fun invoke(target: Any, method: String, paramTypes: Array<Class<*>>, args: Array<Any?>) {
        target.javaClass.getDeclaredMethod(method, *paramTypes).invoke(target, *args)
    }
}
