// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Topic bulletin board via mesh-sdk Publish/Subscribe.

package net.quakemesh.meshapps

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.widget.Toolbar
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.meshapps.ui.DiscussLine
import net.quakemesh.meshapps.ui.DiscussPostsAdapter
import net.quakemesh.sdk.HttpMeshClient
import net.quakemesh.sdk.Session
import org.json.JSONObject
import kotlin.concurrent.thread

class DiscussActivity : AppCompatActivity() {
    private lateinit var client: HttpMeshClient
    private lateinit var session: Session
    private lateinit var topicInput: EditText
    private lateinit var topicStatus: TextView
    private lateinit var postInput: EditText
    private val adapter = DiscussPostsAdapter()
    private var localNodeHex: String = ""
    private var subscribeThread: Thread? = null
    @Volatile private var listening = false
    private var activeTopic: String = DEFAULT_TOPIC

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (!MeshTransport.meshAvailable()) {
            Toast.makeText(this, R.string.mesh_app_required, Toast.LENGTH_LONG).show()
            finish()
            return
        }
        setContentView(R.layout.activity_discuss)
        setupToolbar()

        topicInput = findViewById(R.id.topic_input)
        topicStatus = findViewById(R.id.topic_status)
        postInput = findViewById(R.id.post_input)
        topicInput.setText(DEFAULT_TOPIC)

        findViewById<RecyclerView>(R.id.posts_list).apply {
            layoutManager = LinearLayoutManager(this@DiscussActivity)
            adapter = this@DiscussActivity.adapter
        }

        client = MeshTransport.newClient()
        findViewById<Button>(R.id.subscribe_button).setOnClickListener { subscribeTopic() }
        findViewById<Button>(R.id.post_button).setOnClickListener { publishPost() }

        thread(name = "DiscussRegister") {
            try {
                session = client.register(APP_ID, APP_NAME, APP_VERSION, listOf("pubsub"))
                localNodeHex = MeshTransport.bytesToHex(session.nodeId)
                runOnUiThread { subscribeTopic() }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, getString(R.string.mesh_app_register_failed, e.message), Toast.LENGTH_LONG).show()
                    finish()
                }
            }
        }
    }

    override fun onDestroy() {
        stopSubscribe()
        super.onDestroy()
    }

    override fun onSupportNavigateUp(): Boolean {
        finish()
        return true
    }

    private fun setupToolbar() {
        setSupportActionBar(findViewById<Toolbar>(R.id.toolbar))
        supportActionBar?.setDisplayHomeAsUpEnabled(true)
        title = getString(R.string.discuss_title)
    }

    private fun subscribeTopic() {
        val topic = topicInput.text.toString().trim().ifBlank { DEFAULT_TOPIC }
        activeTopic = topic
        topicStatus.text = getString(R.string.discuss_listening, topic)
        adapter.submit(emptyList())
        stopSubscribe()
        listening = true
        subscribeThread = thread(name = "DiscussSubscribe") {
            try {
                for (payload in client.subscribe(session, topic)) {
                    if (!listening) break
                    val line = parsePost(payload) ?: continue
                    runOnUiThread { adapter.append(line) }
                }
            } catch (_: InterruptedException) {
            } catch (e: Exception) {
                if (listening) {
                    runOnUiThread {
                        Toast.makeText(this, getString(R.string.discuss_subscribe_error, e.message), Toast.LENGTH_SHORT).show()
                    }
                }
            }
        }
    }

    private fun stopSubscribe() {
        listening = false
        subscribeThread?.interrupt()
        subscribeThread = null
    }

    private fun parsePost(payload: ByteArray): DiscussLine? {
        return try {
            val json = JSONObject(String(payload, Charsets.UTF_8))
            val text = json.optString("text", "")
            if (text.isBlank()) return null
            val author = json.optString("author", "?")
            val topic = json.optString("topic", activeTopic)
            DiscussLine(
                authorLabel = MeshTransport.shortId(author),
                topic = topic,
                text = text,
            )
        } catch (_: Exception) {
            DiscussLine("?", activeTopic, String(payload, Charsets.UTF_8))
        }
    }

    private fun publishPost() {
        val text = postInput.text.toString().trim()
        if (text.isEmpty()) return
        val topic = topicInput.text.toString().trim().ifBlank { DEFAULT_TOPIC }
        postInput.text.clear()
        thread(name = "DiscussPublish") {
            try {
                val payload = JSONObject()
                    .put("topic", topic)
                    .put("text", text)
                    .put("author", localNodeHex)
                    .toString()
                    .toByteArray()
                client.publish(session, topic, payload)
                runOnUiThread { adapter.append(DiscussLine("You", topic, text)) }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, getString(R.string.discuss_post_failed, e.message), Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    companion object {
        private const val APP_ID = "net.quakemesh.discuss"
        private const val APP_NAME = "Discuss"
        private const val APP_VERSION = "0.1.0"
        private const val DEFAULT_TOPIC = "general"
    }
}
