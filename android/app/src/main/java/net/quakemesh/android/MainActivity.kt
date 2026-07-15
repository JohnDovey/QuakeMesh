// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: mesh start/stop UI wired to foreground service.
//   0.0.18 - version label, nearby peers drawer, hub override at bottom.
//   0.0.26 - drawer entries for Private Chat and Discuss mesh apps.

package net.quakemesh.android

import android.content.Intent
import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.ImageButton
import android.widget.TextView
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.view.GravityCompat
import androidx.drawerlayout.widget.DrawerLayout
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import androidx.appcompat.app.AlertDialog
import net.quakemesh.meshapps.DiscussActivity
import net.quakemesh.meshapps.PrivateChatActivity
import net.quakemesh.meshapps.R as MeshAppsR
import net.quakemesh.android.mesh.MeshDiscovery
import net.quakemesh.android.mesh.MeshEngine
import net.quakemesh.android.mesh.NodeDisplay
import net.quakemesh.android.mesh.NodeProfile
import net.quakemesh.android.mesh.SosBeacon
import net.quakemesh.android.ui.PeersAdapter
import kotlin.concurrent.thread

class MainActivity : AppCompatActivity() {

    private lateinit var drawerLayout: DrawerLayout
    private lateinit var statusView: TextView
    private lateinit var nodeIdView: TextView
    private lateinit var versionView: TextView
    private lateinit var toggleButton: Button
    private lateinit var sosButton: Button
    private lateinit var hubUrlField: EditText
    private lateinit var handleField: EditText
    private lateinit var homeLatField: EditText
    private lateinit var homeLonField: EditText
    private lateinit var peersList: RecyclerView
    private lateinit var peersEmpty: TextView
    private val peersAdapter = PeersAdapter(
        localNodeId = { MeshEngine.nodeId() },
        hubUrl = { MeshEngine.hubHeartbeatUrl },
        onStatus = { msg -> runOnUiThread { statusView.text = msg } },
    )

    private var meshRunning = false
    private val prefs by lazy { getSharedPreferences("quakemesh_ui", MODE_PRIVATE) }

    private val statusListener: (String) -> Unit = { msg ->
        runOnUiThread {
            val loc = MeshEngine.locationSummary()
            statusView.text = if (loc != null) "$msg\nGPS: $loc" else msg
        }
    }

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
        sosButton = findViewById(R.id.sos_demo_button)
        hubUrlField = findViewById(R.id.hub_heartbeat_url)
        handleField = findViewById(R.id.node_handle)
        homeLatField = findViewById(R.id.home_lat)
        homeLonField = findViewById(R.id.home_lon)
        peersList = findViewById(R.id.peers_list)
        peersEmpty = findViewById(R.id.peers_empty)

        findViewById<ImageButton>(R.id.open_drawer_button).setOnClickListener {
            drawerLayout.openDrawer(GravityCompat.START)
        }
        findViewById<Button>(R.id.nav_private_chat).setOnClickListener {
            openMeshApp(PrivateChatActivity::class.java)
        }
        findViewById<Button>(R.id.nav_discuss).setOnClickListener {
            openMeshApp(DiscussActivity::class.java)
        }

        versionView.text = getString(R.string.version_label, BuildConfig.VERSION_NAME)
        hubUrlField.setText(prefs.getString(PREF_HUB_URL, ""))
        handleField.setText(prefs.getString(NodeProfile.PREF_HANDLE, ""))
        homeLatField.setText(prefs.getString(NodeProfile.PREF_HOME_LAT, ""))
        homeLonField.setText(prefs.getString(NodeProfile.PREF_HOME_LON, ""))
        listOf(handleField, homeLatField, homeLonField).forEach { field ->
            field.setOnFocusChangeListener { _, hasFocus ->
                if (!hasFocus) saveProfile()
            }
        }

        peersList.layoutManager = LinearLayoutManager(this)
        peersList.adapter = peersAdapter

        MeshDiscovery.listener = {
            runOnUiThread { refreshPeersList() }
        }

        toggleButton.setOnClickListener {
            if (meshRunning) {
                MeshForegroundService.stop(this)
                meshRunning = false
                refreshUi()
            } else {
                requestPermissionsIfNeeded()
                saveProfile()
                val hubUrl = hubUrlField.text.toString().trim()
                prefs.edit().putString(PREF_HUB_URL, hubUrl).apply()
                MeshEngine.prepareStart(hubUrl)
                MeshForegroundService.start(this)
                meshRunning = true
                refreshUi()
            }
        }

        sosButton.setOnClickListener {
            if (!meshRunning) {
                statusView.text = getString(R.string.sos_mesh_required)
                return@setOnClickListener
            }
            AlertDialog.Builder(this)
                .setTitle(R.string.sos_confirm_title)
                .setMessage(R.string.sos_confirm_message)
                .setNegativeButton(android.R.string.cancel, null)
                .setPositiveButton(R.string.sos_send) { _, _ -> sendSos() }
                .show()
        }

        refreshUi()
        refreshPeersList()
    }

    override fun onStart() {
        super.onStart()
        MeshEngine.addStatusListener(statusListener)
        meshRunning = MeshEngine.isRunning
        refreshUi()
    }

    override fun onStop() {
        MeshEngine.removeStatusListener(statusListener)
        super.onStop()
    }

    override fun onBackPressed() {
        if (drawerLayout.isDrawerOpen(GravityCompat.START)) {
            drawerLayout.closeDrawer(GravityCompat.START)
        } else {
            super.onBackPressed()
        }
    }

    private fun sendSos() {
        sosButton.isEnabled = false
        statusView.text = getString(R.string.sos_sending)
        thread(name = "SosBeacon") {
            val lines = StringBuilder()
            val ok = SosBeacon.send(
                getString(R.string.sos_message),
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

    private fun refreshPeersList() {
        val peers = MeshDiscovery.peers()
        peersAdapter.submit(peers)
        peersEmpty.visibility = if (peers.isEmpty()) View.VISIBLE else View.GONE
    }

    private fun openMeshApp(activityClass: Class<*>) {
        if (!MeshEngine.isRunning) {
            AlertDialog.Builder(this)
                .setMessage(MeshAppsR.string.mesh_app_required)
                .setPositiveButton(android.R.string.ok, null)
                .show()
            return
        }
        drawerLayout.closeDrawer(GravityCompat.START)
        startActivity(Intent(this, activityClass))
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

    private fun saveProfile() {
        val handle = handleField.text.toString().trim().ifEmpty { null }
        val homeLat = homeLatField.text.toString().trim().toDoubleOrNull()
        val homeLon = homeLonField.text.toString().trim().toDoubleOrNull()
        NodeProfile.save(this, handle, homeLat, homeLon)
    }

    private fun refreshUi() {
        val nodeId = MeshEngine.nodeId()
        nodeIdView.text = if (nodeId != null) {
            getString(
                R.string.node_id_label,
                NodeDisplay.label(handleField.text.toString().trim().ifEmpty { null }, nodeId),
            )
        } else {
            getString(R.string.node_id_unknown)
        }
        toggleButton.text = if (meshRunning) {
            getString(R.string.stop_mesh)
        } else {
            getString(R.string.start_mesh)
        }
        if (!meshRunning) {
            statusView.text = getString(R.string.mesh_stopped)
        } else if (!MeshEngine.isRunning) {
            statusView.text = getString(R.string.mesh_starting)
        }
    }

    companion object {
        private const val PREF_HUB_URL = "hub_heartbeat_url"
    }
}
