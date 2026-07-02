// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.22 - POST trust endorsement to QuakeMeshHub for a nearby peer.

package net.quakemesh.android.mesh

import android.util.Log
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

/** Records a trust endorsement for a peer node via the hub heartbeat API. */
object EndorsePeer {
    private const val TAG = "EndorsePeer"

    fun send(
        endorserNodeIdHex: String,
        endorsedNodeIdHex: String,
        hubBaseUrl: String,
        onLine: (String) -> Unit,
    ) {
        val base = hubBaseUrl.trimEnd('/')
        if (base.isBlank()) {
            onLine("Endorse failed: no hub URL")
            return
        }
        if (endorserNodeIdHex == endorsedNodeIdHex) {
            onLine("Cannot endorse yourself")
            return
        }
        try {
            val body = JSONObject()
                .put("endorser_node_id", endorserNodeIdHex)
                .put("endorsed_node_id", endorsedNodeIdHex)
            val conn = URL("$base/v1/endorse").openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            conn.connectTimeout = 10_000
            conn.readTimeout = 10_000
            conn.outputStream.use { it.write(body.toString().toByteArray()) }
            val code = conn.responseCode
            conn.disconnect()
            if (code in 200..299) {
                onLine("Endorsed ${endorsedNodeIdHex.take(12)}…")
                Log.i(TAG, "endorse ok")
            } else {
                onLine("Endorse HTTP $code")
            }
        } catch (e: Exception) {
            onLine("Endorse failed: ${e.message}")
            Log.w(TAG, "endorse failed", e)
        }
    }
}
