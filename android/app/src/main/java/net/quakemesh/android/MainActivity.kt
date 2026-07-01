// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

package net.quakemesh.android

import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity

/**
 * Scaffold placeholder. Not yet implemented (Phase 4 in /plan.md): BLE +
 * Wi-Fi Direct + Local Only Hotspot transport shims wired to the
 * gomobile-bound /core Go library.
 */
class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
    }
}
