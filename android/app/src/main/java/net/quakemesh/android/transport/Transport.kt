// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: transport interface plugged in by Android shims.

package net.quakemesh.android.transport

interface Transport {
    val name: String
    fun start()
    fun stop()
    fun send(peerHex: String, frame: ByteArray)
}
