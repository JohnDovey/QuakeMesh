// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.19 - Wi-Fi LAN context for infrastructure heartbeats and beacons.

package net.quakemesh.android.mesh

import android.content.Context
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.wifi.WifiManager
import android.os.Build
import androidx.core.content.ContextCompat
import android.Manifest

/**
 * Collects gateway, local IP, SSID, and BSSID for the active Wi-Fi connection.
 * BSSID/SSID may require location permission on Android 10+.
 */
object LanContextCollector {
    data class LanContext(
        val gatewayIp: String,
        val localIp: String? = null,
        val ssid: String? = null,
        val bssid: String? = null,
    )

    fun collect(context: Context): LanContext? {
        val conn = context.applicationContext.getSystemService(ConnectivityManager::class.java)
            ?: return null
        val network = conn.activeNetwork ?: return null
        val link = conn.getLinkProperties(network) ?: return null

        val gateway = link.routes
            .firstOrNull { it.isDefaultRoute && it.gateway != null }
            ?.gateway
            ?.hostAddress
            ?.takeIf { it.isNotBlank() }
            ?: return null

        val localIp = link.linkAddresses
            .firstOrNull { it.address?.isLoopbackAddress == false }
            ?.address
            ?.hostAddress
            ?.takeIf { it.isNotBlank() }

        var ssid: String? = null
        var bssid: String? = null
        if (hasWifiStatePermission(context)) {
            val wifi = context.applicationContext.getSystemService(WifiManager::class.java)
            val info = wifi?.connectionInfo
            ssid = info?.ssid?.trim('"')?.takeIf { it.isNotBlank() && it != "<unknown ssid>" }
            if (hasLocationPermission(context)) {
                bssid = info?.bssid?.takeIf { it.isNotBlank() && it != "02:00:00:00:00:00" }
            }
        }

        return LanContext(
            gatewayIp = gateway,
            localIp = localIp,
            ssid = ssid,
            bssid = bssid,
        )
    }

    private fun hasWifiStatePermission(context: Context): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_WIFI_STATE) ==
            PackageManager.PERMISSION_GRANTED

    private fun hasLocationPermission(context: Context): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return true
        return ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED
    }
}
