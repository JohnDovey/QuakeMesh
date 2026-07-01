// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.10 - Phase 8: periodic GPS fixes for mesh presence (OGM attach later).

package net.quakemesh.android.location

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Bundle
import android.os.Looper
import androidx.core.content.ContextCompat
import java.util.concurrent.atomic.AtomicReference

/**
 * Samples device GPS and exposes the latest fix to the mesh layer.
 * Location will be attached to OGM/presence announcements when the Android
 * node wire-up lands; see Phase 8 in /plan.md.
 */
class LocationReporter(private val context: Context) : LocationListener {

    private val latest = AtomicReference<LocationFix?>(null)
    private var manager: LocationManager? = null

    data class LocationFix(
        val lat: Double,
        val lon: Double,
        val accuracyM: Float,
        val observedAtMs: Long,
    )

    fun start() {
        if (!hasPermission()) return
        val lm = context.getSystemService(Context.LOCATION_SERVICE) as LocationManager
        manager = lm
        val providers = listOf(LocationManager.GPS_PROVIDER, LocationManager.NETWORK_PROVIDER)
            .filter { lm.isProviderEnabled(it) }
        for (provider in providers) {
            runCatching {
                lm.requestLocationUpdates(provider, MIN_INTERVAL_MS, MIN_DISTANCE_M, this, Looper.getMainLooper())
            }
            val last = lm.getLastKnownLocation(provider)
            if (last != null) {
                onLocationChanged(last)
            }
        }
    }

    fun stop() {
        manager?.removeUpdates(this)
        manager = null
    }

    fun latestFix(): LocationFix? = latest.get()

    fun hasPermission(): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED

    override fun onLocationChanged(location: Location) {
        latest.set(
            LocationFix(
                lat = location.latitude,
                lon = location.longitude,
                accuracyM = location.accuracy,
                observedAtMs = location.time,
            ),
        )
    }

    @Deprecated("Deprecated in API")
    override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) {}

    override fun onProviderEnabled(provider: String) {}
    override fun onProviderDisabled(provider: String) {}

    companion object {
        private const val MIN_INTERVAL_MS = 60_000L
        private const val MIN_DISTANCE_M = 25f
    }
}
