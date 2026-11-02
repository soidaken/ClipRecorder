package com.cliprecorder.sync

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.os.Bundle
import android.text.InputType
import android.view.Gravity
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.google.zxing.integration.android.IntentIntegrator

/** Pairing and status screen: paste the line shown by ClipRecorder and start. */
class MainActivity : Activity() {

    private lateinit var input: EditText
    private lateinit var status: TextView
    private lateinit var startButton: Button
    private lateinit var stopButton: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        CrashLog.install(this)

        val density = resources.displayMetrics.density
        fun dp(value: Int) = (value * density).toInt()

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(28), dp(20), dp(20))
            setBackgroundColor(Color.parseColor("#11161d"))
        }

        root.addView(TextView(this).apply {
            text = "ClipSync"
            textSize = 22f
            setTextColor(Color.WHITE)
        })
        root.addView(TextView(this).apply {
            text = getString(R.string.hint_pair)
            textSize = 13f
            setTextColor(Color.parseColor("#a8b3be"))
            setPadding(0, dp(8), 0, dp(10))
        })

        input = EditText(this).apply {
            hint = "192.168.1.5:47820#token"
            textSize = 13f
            setTextColor(Color.WHITE)
            setHintTextColor(Color.parseColor("#6b7783"))
            inputType = InputType.TYPE_CLASS_TEXT
            setSingleLine(true)
            setText(SyncService.prefs(this@MainActivity).getString("pairing_line", ""))
        }
        root.addView(input)

        val buttons = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            setPadding(0, dp(14), 0, dp(10))
        }
        val scanButton = Button(this).apply {
            text = getString(R.string.action_scan)
            setOnClickListener { scanPairingCode() }
        }
        startButton = Button(this).apply {
            text = getString(R.string.action_start)
            setOnClickListener { startSync() }
        }
        stopButton = Button(this).apply {
            text = getString(R.string.action_stop)
            setOnClickListener {
                SyncService.stop(this@MainActivity)
                setStatus(getString(R.string.status_stopped))
            }
        }
        buttons.addView(startButton, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
        buttons.addView(stopButton, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
        buttons.addView(scanButton, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
        root.addView(buttons)

        status = TextView(this).apply {
            textSize = 13f
            setTextColor(Color.parseColor("#a8b3be"))
            gravity = Gravity.START
        }
        root.addView(ScrollView(this).apply { addView(status) })

        root.addView(TextView(this).apply {
            text = getString(R.string.hint_permission)
            textSize = 12f
            setTextColor(Color.parseColor("#6b7783"))
            setPadding(0, dp(16), 0, 0)
        })

        setContentView(root)

        CrashLog.take(this)?.let { report ->
            AlertDialog.Builder(this)
                .setTitle(getString(R.string.crash_title))
                .setMessage(report.take(4000))
                .setPositiveButton(android.R.string.ok) { _, _ ->
                    CrashLog.clear(this)
                }
                .show()
        }
    }

    private fun scanPairingCode() {
        if (checkSelfPermission(Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.CAMERA), CAMERA_REQUEST)
            return
        }
        launchScanner()
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == CAMERA_REQUEST) {
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
                launchScanner()
            } else {
                setStatus(getString(R.string.status_no_camera))
            }
        }
    }

    private fun launchScanner() {
        try {
            startScanner()
        } catch (error: Throwable) {
            setStatus(getString(R.string.status_scan_failed, error.toString()))
        }
    }

    private fun startScanner() {
        IntentIntegrator(this)
            .setDesiredBarcodeFormats(IntentIntegrator.QR_CODE)
            .setPrompt(getString(R.string.scan_prompt))
            .setBeepEnabled(false)
            .setOrientationLocked(false)
            .initiateScan()
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        @Suppress("DEPRECATION")
        super.onActivityResult(requestCode, resultCode, data)
        val result = IntentIntegrator.parseActivityResult(requestCode, resultCode, data) ?: return
        val contents = result.contents
        if (!contents.isNullOrEmpty()) {
            input.setText(contents)
            startSync()
        }
    }

    companion object {
        private const val CAMERA_REQUEST = 1001
    }

    private fun startSync() {
        val line = input.text.toString().trim()
        val pairing = Pairing.parse(line)
        if (pairing == null) {
            setStatus(getString(R.string.status_bad_pairing))
            return
        }
        SyncService.prefs(this).edit().putString("pairing_line", line).apply()
        SyncService.start(this)
        setStatus(getString(R.string.status_started, pairing.host, pairing.port))
    }

    private fun setStatus(text: String) {
        status.text = text
    }
}
