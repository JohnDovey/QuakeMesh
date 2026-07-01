// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: foreground service for continuous mesh participation.

package net.quakemesh.android

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import net.quakemesh.android.mesh.MeshEngine

/**
 * Persistent foreground service required for continuous mesh participation on
 * Android. See "Android Background Execution" in /plan.md.
 */
class MeshForegroundService : Service() {

    override fun onCreate() {
        super.onCreate()
        createChannel()
        MeshEngine.statusListener = { msg ->
            val nm = getSystemService(NotificationManager::class.java)
            nm.notify(NOTIFICATION_ID, buildNotification(msg))
        }
        MeshEngine.start(applicationContext)
        startForeground(NOTIFICATION_ID, buildNotification("QuakeMesh mesh active"))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                MeshEngine.stop()
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        MeshEngine.stop()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun createChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "QuakeMesh mesh",
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = "Shows when this device is participating in the mesh"
        }
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    private fun buildNotification(text: String): Notification {
        val launch = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_sys_download_done)
            .setContentIntent(launch)
            .setOngoing(true)
            .build()
    }

    companion object {
        const val CHANNEL_ID = "quakemesh_mesh"
        const val NOTIFICATION_ID = 42
        const val ACTION_STOP = "net.quakemesh.android.STOP_MESH"

        fun start(context: Context) {
            val intent = Intent(context, MeshForegroundService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        fun stop(context: Context) {
            val intent = Intent(context, MeshForegroundService::class.java).apply {
                action = ACTION_STOP
            }
            context.startService(intent)
        }
    }
}
