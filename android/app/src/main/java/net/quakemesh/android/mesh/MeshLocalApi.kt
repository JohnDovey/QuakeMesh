// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10: loopback HTTP mesh-sdk daemon for Android.

package net.quakemesh.android.mesh

import android.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.ServerSocket
import java.net.Socket
import java.security.SecureRandom
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.Executors
import kotlin.concurrent.thread

/**
 * Minimal loopback HTTP server exposing the same v1 REST API as QuakeMeshHub's
 * mesh-sdk daemon. Third-party apps connect via [net.quakemesh.sdk.HttpMeshClient].
 */
object MeshLocalApi {
    const val DEFAULT_PORT = 18084

    private val executor = Executors.newCachedThreadPool()
    private val sessions = ConcurrentHashMap<String, Session>()
    private val inbox = ConcurrentHashMap<String, CopyOnWriteArrayList<ByteArray>>()
    private val topics = ConcurrentHashMap<String, ConcurrentHashMap<String, CopyOnWriteArrayList<ByteArray>>>()
    private val presence = ConcurrentHashMap<Pair<String, String>, Long>()
    private var serverThread: Thread? = null
    private var nodeIdHex: String = ""

    data class Session(val token: String, val nodeId: String, val appId: String)

    fun start(nodeId: String, port: Int = DEFAULT_PORT) {
        if (serverThread?.isAlive == true) return
        nodeIdHex = nodeId
        serverThread = thread(name = "MeshLocalApi") {
            ServerSocket(port).use { server ->
                while (!Thread.currentThread().isInterrupted) {
                    val socket = server.accept()
                    executor.execute { handle(socket) }
                }
            }
        }
    }

    fun stop() {
        serverThread?.interrupt()
        serverThread = null
        sessions.clear()
        inbox.clear()
        topics.clear()
    }

    fun deliverLocal(payload: ByteArray) {
        val list = inbox.getOrPut(nodeIdHex) { CopyOnWriteArrayList() }
        list.add(payload)
    }

    private fun handle(socket: Socket) {
        socket.use {
            val reader = BufferedReader(InputStreamReader(it.getInputStream()))
            val requestLine = reader.readLine() ?: return
            val parts = requestLine.split(" ")
            if (parts.size < 2) return
            val method = parts[0]
            val path = parts[1]
            val headers = mutableMapOf<String, String>()
            var line: String?
            while (reader.readLine().also { line = it } != null && line!!.isNotEmpty()) {
                val idx = line!!.indexOf(':')
                if (idx > 0) {
                    headers[line!!.substring(0, idx).trim().lowercase()] = line!!.substring(idx + 1).trim()
                }
            }
            val contentLength = headers["content-length"]?.toIntOrNull() ?: 0
            val body = if (contentLength > 0) {
                val buf = CharArray(contentLength)
                reader.read(buf, 0, contentLength)
                String(buf)
            } else ""

            val token = headers["x-mesh-session"].orEmpty()
            val (status, json) = when {
                method == "POST" && path == "/v1/register" -> handleRegister(body)
                method == "POST" && path == "/v1/send" -> handleSend(token, body)
                method == "GET" && path == "/v1/receive" -> handleReceive(token)
                method == "POST" && path == "/v1/publish" -> handlePublish(token, body)
                method == "GET" && path.startsWith("/v1/subscribe") -> handleSubscribe(token, path)
                method == "GET" && path.startsWith("/v1/discover-peers") -> handleDiscover(path)
                else -> 404 to """{"error":"not found"}"""
            }
            val response = "HTTP/1.1 $status\r\nContent-Type: application/json\r\nContent-Length: ${json.length}\r\nConnection: close\r\n\r\n$json"
            it.getOutputStream().write(response.toByteArray())
        }
    }

    private fun handleRegister(body: String): Pair<Int, String> {
        val json = JSONObject(body)
        val appId = json.optString("app_id")
        if (appId.isEmpty()) return 400 to """{"error":"app_id required"}"""
        val token = randomToken()
        val appName = json.optString("app_name")
        val appVersion = json.optString("app_version")
        sessions[token] = Session(token, nodeIdHex, appId)
        presence[appId to appVersion] = System.currentTimeMillis()
        val out = JSONObject()
            .put("session_token", token)
            .put("node_id", nodeIdHex)
        return 200 to out.toString()
    }

    private fun handleSend(token: String, body: String): Pair<Int, String> {
        if (!sessions.containsKey(token)) return 401 to """{"error":"unauthorized"}"""
        val json = JSONObject(body)
        if (!json.has("dest_node_id")) return 400 to """{"error":"dest required"}"""
        // Phase 10 MVP: accept send; mesh routing lands with full node core.
        return 200 to """{"ok":true}"""
    }

    private fun handleReceive(token: String): Pair<Int, String> {
        val sess = sessions[token] ?: return 401 to """{"error":"unauthorized"}"""
        val queue = inbox[sess.nodeId]
        val payload = if (!queue.isNullOrEmpty()) queue.removeAt(0) else null
        if (payload == null) return 204 to ""
        val out = JSONObject().put("payload_b64", Base64.encodeToString(payload, Base64.NO_WRAP))
        return 200 to out.toString()
    }

    private fun handlePublish(token: String, body: String): Pair<Int, String> {
        if (!sessions.containsKey(token)) return 401 to """{"error":"unauthorized"}"""
        val json = JSONObject(body)
        val topic = json.optString("topic")
        val payloadB64 = json.optString("payload_b64")
        if (topic.isEmpty()) return 400 to """{"error":"topic required"}"""
        val payload = Base64.decode(payloadB64, Base64.NO_WRAP)
        val subs = topics.getOrPut(topic) { ConcurrentHashMap() }
        subs.values.forEach { it.add(payload) }
        return 200 to """{"ok":true}"""
    }

    private fun handleSubscribe(token: String, path: String): Pair<Int, String> {
        if (!sessions.containsKey(token)) return 401 to """{"error":"unauthorized"}"""
        val query = path.substringAfter('?', "")
        val topic = query.split('&').mapNotNull {
            val kv = it.split('=')
            if (kv.size == 2 && kv[0] == "topic") kv[1] else null
        }.firstOrNull() ?: return 400 to """{"error":"topic required"}"""
        val bucket = topics.getOrPut(topic) { ConcurrentHashMap() }
            .getOrPut(token) { CopyOnWriteArrayList() }
        val payload = if (bucket.isNotEmpty()) bucket.removeAt(0) else null
        if (payload == null) return 204 to ""
        val out = JSONObject().put("payload_b64", Base64.encodeToString(payload, Base64.NO_WRAP))
        return 200 to out.toString()
    }

    private fun handleDiscover(path: String): Pair<Int, String> {
        val appId = path.substringAfter("app_id=", "").substringBefore('&')
        val peers = JSONArray()
        presence.keys.filter { it.first == appId }.forEach { _ ->
            peers.put(nodeIdHex)
        }
        return 200 to JSONObject().put("peers", peers).toString()
    }

    private fun randomToken(): String {
        val bytes = ByteArray(24)
        SecureRandom().nextBytes(bytes)
        return bytes.joinToString("") { "%02x".format(it) }
    }
}
