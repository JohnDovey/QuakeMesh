// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10: HTTP client for loopback mesh-sdk API.

package net.quakemesh.sdk

import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.util.Base64

/**
 * Talks to QuakeMesh's loopback mesh-sdk daemon ([MeshLocalApi] on Android
 * or QuakeMeshHub's Unix socket via port forward).
 */
class HttpMeshClient(
    private val baseUrl: String = "http://127.0.0.1:18084",
) : MeshClient {
    private var sessionToken: String = ""

    override fun register(
        appId: String,
        appName: String,
        appVersion: String,
        capabilities: List<String>,
    ): Session {
        val body = JSONObject()
            .put("app_id", appId)
            .put("app_name", appName)
            .put("app_version", appVersion)
            .put("capabilities", capabilities)
        val resp = post("/v1/register", body.toString(), null)
        sessionToken = resp.getString("session_token")
        val nodeId = resp.optString("node_id", "").chunked(2)
            .map { it.toInt(16).toByte() }
            .toByteArray()
        return Session(appId, appName, appVersion, nodeId)
    }

    override fun send(session: Session, destNodeId: ByteArray, payload: ByteArray) {
        val body = JSONObject()
            .put("dest_node_id", destNodeId.joinToString("") { "%02x".format(it) })
            .put("payload_b64", Base64.getEncoder().encodeToString(payload))
        post("/v1/send", body.toString(), sessionToken)
    }

    override fun receive(session: Session): Sequence<ByteArray> = sequence {
        while (true) {
            val payload = pollReceive() ?: continue
            yield(payload)
        }
    }

    override fun publish(session: Session, topic: String, payload: ByteArray) {
        val body = JSONObject()
            .put("topic", topic)
            .put("payload_b64", Base64.getEncoder().encodeToString(payload))
        post("/v1/publish", body.toString(), sessionToken)
    }

    override fun subscribe(session: Session, topic: String): Sequence<ByteArray> = sequence {
        while (true) {
            val url = URL("$baseUrl/v1/subscribe?topic=$topic&timeout=25s")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "GET"
            conn.setRequestProperty("X-Mesh-Session", sessionToken)
            if (conn.responseCode == 204) {
                conn.disconnect()
                continue
            }
            val text = conn.inputStream.bufferedReader().readText()
            conn.disconnect()
            val json = JSONObject(text)
            yield(Base64.getDecoder().decode(json.getString("payload_b64")))
        }
    }

    override fun discoverPeers(appId: String, versionConstraint: String?): List<ByteArray> {
        var path = "/v1/discover-peers?app_id=$appId"
        if (!versionConstraint.isNullOrEmpty()) path += "&version_constraint=$versionConstraint"
        val resp = get(path, sessionToken)
        val arr = resp.getJSONArray("peers")
        return (0 until arr.length()).map { i ->
            val s = arr.getString(i)
            s.chunked(2).map { byte -> byte.toInt(16).toByte() }.toByteArray()
        }
    }

    private fun pollReceive(): ByteArray? {
        val conn = URL("$baseUrl/v1/receive").openConnection() as HttpURLConnection
        conn.requestMethod = "GET"
        conn.setRequestProperty("X-Mesh-Session", sessionToken)
        if (conn.responseCode == 204) {
            conn.disconnect()
            Thread.sleep(200)
            return null
        }
        val text = conn.inputStream.bufferedReader().readText()
        conn.disconnect()
        val json = JSONObject(text)
        return Base64.getDecoder().decode(json.getString("payload_b64"))
    }

    private fun post(path: String, body: String, token: String?): JSONObject {
        val conn = URL("$baseUrl$path").openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.setRequestProperty("Content-Type", "application/json")
        if (token != null) conn.setRequestProperty("X-Mesh-Session", token)
        conn.outputStream.use { it.write(body.toByteArray()) }
        val text = conn.inputStream.bufferedReader().readText()
        conn.disconnect()
        return JSONObject(text)
    }

    private fun get(path: String, token: String): JSONObject {
        val conn = URL("$baseUrl$path").openConnection() as HttpURLConnection
        conn.requestMethod = "GET"
        conn.setRequestProperty("X-Mesh-Session", token)
        val text = conn.inputStream.bufferedReader().readText()
        conn.disconnect()
        return JSONObject(text)
    }
}
