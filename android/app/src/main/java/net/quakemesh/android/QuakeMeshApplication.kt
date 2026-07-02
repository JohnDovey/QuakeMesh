// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>

package net.quakemesh.android

import android.app.Application
import net.quakemesh.android.mesh.MeshEngine
import net.quakemesh.meshapps.MeshTransport

class QuakeMeshApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        MeshTransport.availabilityCheck = { MeshEngine.isRunning }
    }
}
