// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// 1:1 private chat via mesh-sdk Send/Receive.

package net.quakemesh.meshapps

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.widget.Toolbar
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.meshapps.ui.ChatLine
import net.quakemesh.meshapps.ui.ChatMessagesAdapter
import net.quakemesh.sdk.HttpMeshClient
import net.quakemesh.sdk.Session
import org.json.JSONObject
import kotlin.concurrent.thread

class PrivateChatActivity : AppCompatActivity() {
    private lateinit var client: HttpMeshClient
    private lateinit var session: Session
    private lateinit var peerLabel: TextView
    private lateinit var messageInput: EditText
    private val adapter = ChatMessagesAdapter()
    private var selectedPeerHex: String? = null
    private var localNodeHex: String = ""
    private var receiveThread: Thread? = null
    @Volatile private var listening = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (!MeshTransport.meshAvailable()) {
            Toast.makeText(this, R.string.mesh_app_required, Toast.LENGTH_LONG).show()
            finish()
            return
        }
        setContentView(R.layout.activity_private_chat)
        setupToolbar()

        peerLabel = findViewById(R.id.peer_label)
        messageInput = findViewById(R.id.message_input)
        findViewById<RecyclerView>(R.id.messages_list).apply {
            layoutManager = LinearLayoutManager(this@PrivateChatActivity).apply { stackFromEnd = true }
            adapter = this@PrivateChatActivity.adapter
        }

        client = MeshTransport.newClient()
        findViewById<Button>(R.id.pick_peer_button).setOnClickListener { pickPeer() }
        findViewById<Button>(R.id.send_button).setOnClickListener { sendMessage() }

        thread(name = "PrivateChatRegister") {
            try {
                session = client.register(APP_ID, APP_NAME, APP_VERSION, listOf("messaging"))
                localNodeHex = MeshTransport.bytesToHex(session.nodeId)
                runOnUiThread {
                    startReceiveLoop()
                    pickPeer()
                }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, getString(R.string.mesh_app_register_failed, e.message), Toast.LENGTH_LONG).show()
                    finish()
                }
            }
        }
    }

    override fun onDestroy() {
        listening = false
        receiveThread?.interrupt()
        super.onDestroy()
    }

    override fun onSupportNavigateUp(): Boolean {
        finish()
        return true
    }

    private fun setupToolbar() {
        setSupportActionBar(findViewById<Toolbar>(R.id.toolbar))
        supportActionBar?.setDisplayHomeAsUpEnabled(true)
        title = getString(R.string.private_chat_title)
    }

    private fun startReceiveLoop() {
        listening = true
        receiveThread = thread(name = "PrivateChatRecv") {
            try {
                for (payload in client.receive(session)) {
                    if (!listening) break
                    val line = parseIncoming(payload) ?: continue
                    runOnUiThread { adapter.append(line) }
                }
            } catch (_: InterruptedException) {
            } catch (e: Exception) {
                if (listening) {
                    runOnUiThread {
                        Toast.makeText(this, getString(R.string.private_chat_receive_error, e.message), Toast.LENGTH_SHORT).show()
                    }
                }
            }
        }
    }

    private fun parseIncoming(payload: ByteArray): ChatLine? {
        return try {
            val json = JSONObject(String(payload, Charsets.UTF_8))
            val text = json.optString("text", "")
            if (text.isBlank()) return null
            val sender = json.optString("sender", "?")
            ChatLine(
                senderLabel = MeshTransport.shortId(sender),
                text = text,
                outgoing = sender.equals(localNodeHex, ignoreCase = true),
            )
        } catch (_: Exception) {
            ChatLine("?", String(payload, Charsets.UTF_8), outgoing = false)
        }
    }

    private fun pickPeer() {
        thread(name = "PrivateChatDiscover") {
            try {
                val peers = client.discoverPeers(APP_ID, null)
                    .map { MeshTransport.bytesToHex(it) }
                    .filter { !it.equals(localNodeHex, ignoreCase = true) }
                runOnUiThread { showPeerPicker(peers) }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, getString(R.string.private_chat_discover_failed, e.message), Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    private fun showPeerPicker(peers: List<String>) {
        if (peers.isEmpty()) {
            AlertDialog.Builder(this)
                .setTitle(R.string.private_chat_pick_peer)
                .setMessage(R.string.private_chat_no_peers)
                .setPositiveButton(android.R.string.ok, null)
                .show()
            return
        }
        val labels = peers.map { MeshTransport.shortId(it) }.toTypedArray()
        AlertDialog.Builder(this)
            .setTitle(R.string.private_chat_pick_peer)
            .setItems(labels) { _, which ->
                selectedPeerHex = peers[which]
                peerLabel.text = getString(R.string.private_chat_peer_selected, labels[which])
            }
            .show()
    }

    private fun sendMessage() {
        val dest = selectedPeerHex
        if (dest.isNullOrBlank()) {
            Toast.makeText(this, R.string.private_chat_pick_peer_first, Toast.LENGTH_SHORT).show()
            return
        }
        val text = messageInput.text.toString().trim()
        if (text.isEmpty()) return
        messageInput.text.clear()
        thread(name = "PrivateChatSend") {
            try {
                val payload = JSONObject()
                    .put("text", text)
                    .put("sender", localNodeHex)
                    .toString()
                    .toByteArray()
                client.send(session, MeshTransport.hexToBytes(dest), payload)
                runOnUiThread { adapter.append(ChatLine("You", text, outgoing = true)) }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, getString(R.string.private_chat_send_failed, e.message), Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    companion object {
        private const val APP_ID = "net.quakemesh.privatechat"
        private const val APP_NAME = "Private Chat"
        private const val APP_VERSION = "0.1.0"
    }
}
