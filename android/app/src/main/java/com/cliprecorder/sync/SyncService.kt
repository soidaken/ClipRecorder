package com.cliprecorder.sync

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Keeps the phone and the PC in sync while it runs: long polls the PC and
 * watches the phone clipboard. Android only lets the foreground app (or an app
 * with the READ_CLIPBOARD/WRITE_CLIPBOARD appops granted) touch the clipboard,
 * so the service reports what it could not do instead of failing silently.
 */
class SyncService : Service() {

    private var worker: Thread? = null
    private val running = AtomicBoolean(false)

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val line = intent?.getStringExtra(EXTRA_PAIRING) ?: prefs(this).getString(KEY_PAIRING, null)
        val pairing = line?.let(Pairing::parse)
        if (pairing == null) {
            stopSelf()
            return START_NOT_STICKY
        }
        startForeground(NOTIFICATION_ID, notification(getString(R.string.status_connecting)))
        if (!running.compareAndSet(false, true)) return START_STICKY

        val clipboard = PhoneClipboard(this)
        val client = ClipClient(pairing)
        worker = Thread({
            var lastRev = 0L
            var lastApplied = ""
            var lastPushed = ""
            var lastWatch = 0L
            while (running.get()) {
                // Phone -> PC: compare the clipboard every second.
                val current = clipboard.read()
                if (current != null && current.isNotEmpty() && current != lastApplied && current != lastPushed) {
                    if (client.push(current)) {
                        lastPushed = current
                        lastRev = (lastRev + 1)
                    }
                }
                // PC -> phone, blocking up to ~20 s.
                val result = client.fetch(lastRev)
                if (!running.get()) break
                if (result.ok) {
                    lastRev = result.rev
                    if (result.text.isNotEmpty() && result.text != lastPushed && result.text != lastApplied) {
                        if (clipboard.write(result.text)) {
                            lastApplied = result.text
                        } else {
                            notify(getString(R.string.status_no_permission))
                        }
                    }
                }
                val now = System.currentTimeMillis()
                if (now - lastWatch > 4000) {
                    lastWatch = now
                    updateNotification(getString(R.string.status_running))
                }
            }
        }, "clipsync").apply { isDaemon = true }
        worker?.start()
        updateNotification(getString(R.string.status_running))
        return START_STICKY
    }

    override fun onDestroy() {
        running.set(false)
        worker?.interrupt()
        super.onDestroy()
    }

    private fun notification(text: String): Notification {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(CHANNEL_ID, "ClipSync", NotificationManager.IMPORTANCE_LOW)
            manager.createNotificationChannel(channel)
        }
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, CHANNEL_ID)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        return builder
            .setContentTitle("ClipSync")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentIntent(open)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(text: String) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, notification(text))
    }

    private fun notify(text: String) = updateNotification(text)

    companion object {
        const val EXTRA_PAIRING = "pairing"
        private const val CHANNEL_ID = "clipsync"
        private const val NOTIFICATION_ID = 42
        private const val KEY_PAIRING = "pairing_line"

        fun prefs(context: Context) = context.getSharedPreferences("clipsync", Context.MODE_PRIVATE)

        fun start(context: Context) {
            val intent = Intent(context, SyncService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, SyncService::class.java))
        }
    }
}
