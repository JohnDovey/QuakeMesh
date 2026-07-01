// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: mesh start/stop UI wired to foreground service.
//   0.0.10 - Phase 8: show GPS fix when mesh is running.

package net.quakemesh.android

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.widget.Button
import android.widget.TextView
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import net.quakemesh.android.mesh.MeshEngine

class MainActivity : AppCompatActivity() {

    private lateinit var statusView: TextView
    private lateinit var nodeIdView: TextView
    private lateinit var toggleButton: Button

    private var meshRunning = false

    private val permissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { _ ->
        refreshUi()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusView = findViewById(R.id.status_text)
        nodeIdView = findViewById(R.id.node_id_text)
        toggleButton = findViewById(R.id.toggle_mesh_button)

        MeshEngine.statusListener = { msg ->
            runOnUiThread {
                val loc = MeshEngine.locationSummary()
                statusView.text = if (loc != null) "$msg\nGPS: $loc" else msg
            }
        }

        toggleButton.setOnClickListener {
            if (meshRunning) {
                MeshForegroundService.stop(this)
                meshRunning = false
            } else {
                requestPermissionsIfNeeded()
                MeshForegroundService.start(this)
                meshRunning = true
            }
            refreshUi()
        }

        refreshUi()
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
}
