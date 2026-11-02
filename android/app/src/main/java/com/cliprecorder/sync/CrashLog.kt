package com.cliprecorder.sync

import android.content.Context
import java.io.File
import java.io.PrintWriter
import java.io.StringWriter

/** Keeps the last crash on disk so the next launch can show what happened. */
object CrashLog {
    private fun file(context: Context) = File(context.filesDir, "last-crash.txt")

    fun install(context: Context) {
        val previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, error ->
            runCatching {
                val text = StringWriter()
                error.printStackTrace(PrintWriter(text))
                file(context).writeText(text.toString())
            }
            previous?.uncaughtException(thread, error)
        }
    }

    fun take(context: Context): String? {
        val target = file(context)
        if (!target.exists()) return null
        return runCatching { target.readText() }.getOrNull()
    }

    fun clear(context: Context) {
        runCatching { file(context).delete() }
    }
}
