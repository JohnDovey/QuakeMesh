// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Phase 12: exercises HttpMeshClient against the loopback mesh-sdk daemon.

package net.quakemesh.android.demo

import net.quakemesh.android.mesh.MeshLocalApi
import net.quakemesh.sdk.HttpMeshClient
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.concurrent.thread

object ReferenceSdkDemo {

    /** Runs register + publish/subscribe smoke test. Returns false on failure. */
    fun run(onLine: (String) -> Unit): Boolean {
        return try {
            val base = "http://127.0.0.1:${MeshLocalApi.DEFAULT_PORT}"
            val pub = HttpMeshClient(base)
            val sess = pub.register(
                "net.quakemesh.meshdemo",
                "Mesh Demo",
                "0.1.0",
                listOf("demo"),
            )
            val node = sess.nodeId.joinToString("") { "%02x".format(it) }
            onLine("register ok node=${node.take(16)}…")

            val sub = HttpMeshClient(base)
            sub.register("net.quakemesh.meshdemo-sub", "Sub", "0.1.0", emptyList())
            val topic = "meshdemo-smoke"
            val latch = CountDownLatch(1)
            var payload = ""
            thread(name = "SdkDemoSubscribe") {
                for (p in sub.subscribe(sess, topic)) {
                    payload = String(p)
                    latch.countDown()
                    break
                }
            }
            Thread.sleep(100)
            pub.publish(sess, topic, "meshdemo-ping".toByteArray())
            if (!latch.await(5, TimeUnit.SECONDS)) {
                onLine("subscribe timed out (start mesh first)")
                return false
            }
            onLine("publish/subscribe ok payload=$payload")
            true
        } catch (e: Exception) {
            onLine("sdk demo failed: ${e.message}")
            false
        }
    }
}
