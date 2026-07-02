// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Standalone launcher for mesh-sdk reference apps (Private Chat + Discuss).

package net.quakemesh.meshapps.standalone

import android.content.Intent
import android.os.Bundle
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import net.quakemesh.meshapps.DiscussActivity
import net.quakemesh.meshapps.MeshTransport
import net.quakemesh.meshapps.PrivateChatActivity

class MeshAppsLauncherActivity : AppCompatActivity() {
    private lateinit var transportStatus: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_launcher)
        title = getString(R.string.app_name)

        findViewById<TextView>(R.id.version_text).text =
            getString(R.string.version_label, BuildConfig.VERSION_NAME)
        transportStatus = findViewById(R.id.transport_status)

        findViewById<Button>(R.id.open_private_chat).setOnClickListener {
            openMeshApp(PrivateChatActivity::class.java)
        }
        findViewById<Button>(R.id.open_discuss).setOnClickListener {
            openMeshApp(DiscussActivity::class.java)
        }
    }

    override fun onResume() {
        super.onResume()
        refreshTransportStatus()
    }

    private fun refreshTransportStatus() {
        val ok = MeshTransport.meshAvailable()
        transportStatus.text = if (ok) {
            getString(R.string.launcher_transport_ok)
        } else {
            getString(R.string.launcher_transport_missing)
        }
    }

    private fun openMeshApp(activityClass: Class<*>) {
        if (!MeshTransport.meshAvailable()) {
            AlertDialog.Builder(this)
                .setMessage(net.quakemesh.meshapps.R.string.mesh_app_required)
                .setPositiveButton(android.R.string.ok, null)
                .show()
            return
        }
        startActivity(Intent(this, activityClass))
    }
}
