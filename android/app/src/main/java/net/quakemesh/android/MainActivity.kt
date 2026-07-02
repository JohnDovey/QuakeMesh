// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: mesh start/stop UI wired to foreground service.
//   0.0.18 - version label, nearby peers drawer, hub override at bottom.

package net.quakemesh.android

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.view.GravityCompat
import androidx.drawerlayout.widget.DrawerLayout
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.android.demo.ReferenceSdkDemo
import net.quakemesh.android.demo.SosBeaconDemo
import net.quakemesh.android.mesh.MeshDiscovery
import net.quakemesh.android.mesh.MeshEngine
import net.quakemesh.android.ui.PeersAdapter
import kotlin.concurrent.thread

class MainActivity : AppCompatActivity() {

    private lateinit var drawerLayout: DrawerLayout
    private lateinit var statusView: TextView
    private lateinit var nodeIdView: TextView
    private lateinit var versionView: TextView
    private lateinit var toggleButton: Button
    private lateinit var sdkDemoButton: Button
    private lateinit var sosButton: Button
    private lateinit var hubUrlField: EditText
    private lateinit var peersList: RecyclerView
    private lateinit var peersEmpty: TextView
    private val peersAdapter = PeersAdapter()

    private var meshRunning = false
    private val prefs by lazy { getSharedPreferences("quakemesh_ui", MODE_PRIVATE) }

    private val permissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { _ ->
        refreshUi()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        drawerLayout = findViewById(R.id.drawer_layout)
        statusView = findViewById(R.id.status_text)
        nodeIdView = findViewById(R.id.node_id_text)
        versionView = findViewById(R.id.version_text)
        toggleButton = findViewById(R.id.toggle_mesh_button)
        sdkDemoButton = findViewById(R.id.sdk_demo_button)
        sosButton = findViewById(R.id.sos_demo_button)
        hubUrlField = findViewById(R.id.hub_heartbeat_url)
        peersList = findViewById(R.id.peers_list)
        peersEmpty = findViewById(R.id.peers_empty)

        versionView.text = getString(R.string.version_label, BuildConfig.VERSION_NAME)
        hubUrlField.setText(prefs.getString(PREF_HUB_URL, ""))

        peersList.layoutManager = LinearLayoutManager(this)
        peersList.adapter = peersAdapter

        MeshEngine.statusListener = { msg ->
            runOnUiThread {
                val loc = MeshEngine.locationSummary()
                statusView.text = if (loc != null) "$msg\nGPS: $loc" else msg
            }
        }

        MeshDiscovery.listener = {
            runOnUiThread { refreshPeersList() }
        }

        toggleButton.setOnClickListener {
            if (meshRunning) {
                MeshForegroundService.stop(this)
                meshRunning = false
            } else {
                requestPermissionsIfNeeded()
                val hubUrl = hubUrlField.text.toString().trim()
                prefs.edit().putString(PREF_HUB_URL, hubUrl).apply()
                MeshEngine.prepareStart(hubUrl)
                MeshForegroundService.start(this)
                meshRunning = true
            }
            refreshUi()
        }

        sdkDemoButton.setOnClickListener {
            if (!meshRunning) {
                statusView.text = getString(R.string.sdk_demo_mesh_required)
                return@setOnClickListener
            }
            sdkDemoButton.isEnabled = false
            statusView.text = getString(R.string.sdk_demo_running)
            thread(name = "SdkDemo") {
                val lines = StringBuilder()
                val ok = ReferenceSdkDemo.run { line ->
                    lines.append(line).append('\n')
                    runOnUiThread { statusView.text = lines.toString() }
                }
                runOnUiThread {
                    sdkDemoButton.isEnabled = true
                    if (ok) {
                        lines.append(getString(R.string.sdk_demo_ok))
                        statusView.text = lines.toString()
                    }
                }
            }
        }

        sosButton.setOnClickListener {
            if (!meshRunning) {
                statusView.text = getString(R.string.sos_mesh_required)
                return@setOnClickListener
            }
            sosButton.isEnabled = false
            statusView.text = getString(R.string.sos_sending)
            thread(name = "SosBeacon") {
                val lines = StringBuilder()
                val ok = SosBeaconDemo.send(
                    getString(R.string.sos_test_message),
                    MeshEngine.latestLocation(),
                ) { line ->
                    lines.append(line).append('\n')
                    runOnUiThread { statusView.text = lines.toString() }
                }
                runOnUiThread {
                    sosButton.isEnabled = true
                    if (ok) {
                        lines.append(getString(R.string.sos_sent_ok))
                        statusView.text = lines.toString()
                    }
                }
            }
        }

        refreshUi()
        refreshPeersList()
    }

    override fun onBackPressed() {
        if (drawerLayout.isDrawerOpen(GravityCompat.START)) {
            drawerLayout.closeDrawer(GravityCompat.START)
        } else {
            super.onBackPressed()
        }
    }

    private fun refreshPeersList() {
        val peers = MeshDiscovery.peers()
        peersAdapter.submit(peers)
        peersEmpty.visibility = if (peers.isEmpty()) View.VISIBLE else View.GONE
    }

    private fun requestPermissionsIfNeeded() {
        val needed = requiredPermissions().filter {
            ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED
        }
        if (needed.isNotEmpty()) {
            permissionLauncher.launch(needed.toTypedArray())
        }
    }

    private fun requiredPermissions(): List<String> {
        val perms = mutableListOf(
            Manifest.permission.ACCESS_FINE_LOCATION,
            Manifest.permission.ACCESS_WIFI_STATE,
            Manifest.permission.CHANGE_WIFI_MULTICAST_STATE,
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            perms += Manifest.permission.BLUETOOTH_SCAN
            perms += Manifest.permission.BLUETOOTH_ADVERTISE
            perms += Manifest.permission.BLUETOOTH_CONNECT
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            perms += Manifest.permission.NEARBY_WIFI_DEVICES
        }
        return perms
    }

    private fun refreshUi() {
        nodeIdView.text = getString(
            R.string.node_id_label,
            MeshEngine.nodeId()?.take(16) ?: getString(R.string.node_id_unknown),
        )
        toggleButton.text = if (meshRunning) {
            getString(R.string.stop_mesh)
        } else {
            getString(R.string.start_mesh)
        }
        if (!meshRunning) {
            statusView.text = getString(R.string.mesh_stopped)
        }
    }

    companion object {
        private const val PREF_HUB_URL = "hub_heartbeat_url"
    }
}
