// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   1.0.1 - User handle and home location profile fields.

package net.quakemesh.android.mesh

import android.content.Context

/** Local user profile sent to the hub on heartbeat and LAN beacons. */
object NodeProfile {
    const val PREFS = "quakemesh_ui"
    const val PREF_HANDLE = "node_handle"
    const val PREF_HOME_LAT = "home_lat"
    const val PREF_HOME_LON = "home_lon"

    data class Profile(
        val handle: String?,
        val homeLat: Double?,
        val homeLon: Double?,
    )

    fun load(context: Context): Profile {
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val handle = prefs.getString(PREF_HANDLE, null)?.trim()?.takeIf { it.isNotEmpty() }
        val homeLat = prefs.getString(PREF_HOME_LAT, null)?.toDoubleOrNull()
        val homeLon = prefs.getString(PREF_HOME_LON, null)?.toDoubleOrNull()
        val home = if (homeLat != null && homeLon != null) homeLat to homeLon else null
        return Profile(handle, home?.first, home?.second)
    }

    fun save(context: Context, handle: String?, homeLat: Double?, homeLon: Double?) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putString(PREF_HANDLE, handle?.trim().orEmpty())
            .putString(PREF_HOME_LAT, homeLat?.toString().orEmpty())
            .putString(PREF_HOME_LON, homeLon?.toString().orEmpty())
            .apply()
    }
}

object NodeDisplay {
    fun shortId(hex: String): String =
        if (hex.length > 16) hex.take(16) + "…" else hex

    fun label(handle: String?, nodeId: String): String {
        val id = shortId(nodeId)
        return if (!handle.isNullOrBlank()) "${handle.trim()} ($id)" else id
    }
}
