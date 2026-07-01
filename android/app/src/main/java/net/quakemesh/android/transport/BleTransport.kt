// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: BLE transport skeleton (advertise + scan service UUID).

package net.quakemesh.android.transport

import android.bluetooth.BluetoothAdapter
import android.bluetooth.le.AdvertiseCallback
import android.bluetooth.le.AdvertiseData
import android.bluetooth.le.AdvertiseSettings
import android.bluetooth.le.BluetoothLeAdvertiser
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.os.ParcelUuid
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Bluetooth LE fallback transport skeleton. Phase 4 starts advertising and
 * scanning the QuakeMesh service UUID; GATT frame exchange is added when
 * multi-hop BLE paths are tuned.
 */
class BleTransport(
    @Suppress("UNUSED_PARAMETER") private val context: Context,
    @Suppress("UNUSED_PARAMETER") private val onFrame: (peerHex: String, frame: ByteArray) -> Unit,
) : Transport {

    override val name: String = "ble"

    private val running = AtomicBoolean(false)
    private var advertiser: BluetoothLeAdvertiser? = null
    private var scanner: BluetoothLeScanner? = null

    private val advertiseCallback = object : AdvertiseCallback() {}
    private val scanCallback = object : ScanCallback() {
        override fun onScanResult(callbackType: Int, result: ScanResult?) {
            // Phase 4: discovery only; connect + GATT in a follow-up.
        }
    }

    override fun start() {
        if (!running.compareAndSet(false, true)) return
        val adapter = BluetoothAdapter.getDefaultAdapter() ?: return
        advertiser = adapter.bluetoothLeAdvertiser
        scanner = adapter.bluetoothLeScanner

        val settings = AdvertiseSettings.Builder()
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_LOW_LATENCY)
            .setConnectable(true)
            .setTimeout(0)
            .setTxPowerLevel(AdvertiseSettings.ADVERTISE_TX_POWER_MEDIUM)
            .build()
        val data = AdvertiseData.Builder()
            .addServiceUuid(ParcelUuid(SERVICE_UUID))
            .setIncludeDeviceName(false)
            .build()
        advertiser?.startAdvertising(settings, data, advertiseCallback)

        val filter = ScanFilter.Builder().setServiceUuid(ParcelUuid(SERVICE_UUID)).build()
        val scanSettings = ScanSettings.Builder()
            .setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY)
            .build()
        scanner?.startScan(listOf(filter), scanSettings, scanCallback)
    }

    override fun stop() {
        if (!running.compareAndSet(true, false)) return
        advertiser?.stopAdvertising(advertiseCallback)
        scanner?.stopScan(scanCallback)
    }

    override fun send(peerHex: String, frame: ByteArray) {
        // Not yet implemented — requires GATT connection per peer.
    }

    companion object {
        val SERVICE_UUID: UUID = UUID.fromString("7a5b1c20-3f4e-4a9b-9c2d-8e1f0a2b3c4d")
    }
}
