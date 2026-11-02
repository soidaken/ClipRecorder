package com.cliprecorder.sync

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import java.io.BufferedReader
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL

/** One pairing line: "192.168.1.5:47820#token". */
data class Pairing(val host: String, val port: Int, val token: String) {
    val baseUrl: String get() = "http://$host:$port"

    companion object {
        fun parse(line: String): Pairing? {
            val text = line.trim()
            val hash = text.lastIndexOf('#')
            if (hash <= 0) return null
            val token = text.substring(hash + 1).trim()
            val address = text.substring(0, hash)
            val colon = address.lastIndexOf(':')
            if (colon <= 0 || token.isEmpty()) return null
            val host = address.substring(0, colon)
            val port = address.substring(colon + 1).toIntOrNull() ?: return null
            return Pairing(host, port, token)
        }
    }
}

/** Long polling client for the PC clipboard. */
class ClipClient(private val pairing: Pairing) {

    /** Waits up to ~20 s for a revision newer than [since]. */
    fun fetch(since: Long): Result {
        var connection: HttpURLConnection? = null
        return try {
            connection = open("/api/clip?since=$since", "GET")
            connection.readTimeout = 25_000
            val body = connection.inputStream.bufferedReader().use(BufferedReader::readText)
            val json = org.json.JSONObject(body)
            Result(json.getLong("rev"), json.optString("text", ""))
        } catch (e: Exception) {
            Result(-1, "")
        } finally {
            connection?.disconnect()
        }
    }

    /** Sends phone clipboard text to the PC. */
    fun push(text: String): Boolean {
        var connection: HttpURLConnection? = null
        return try {
            connection = open("/api/clip", "POST")
            connection.doOutput = true
            val payload = org.json.JSONObject().put("text", text).toString()
            OutputStreamWriter(connection.outputStream, Charsets.UTF_8).use { it.write(payload) }
            connection.responseCode in 200..299
        } catch (e: Exception) {
            false
        } finally {
            connection?.disconnect()
        }
    }

    private fun open(path: String, method: String): HttpURLConnection {
        val url = URL(pairing.baseUrl + path)
        val connection = url.openConnection() as HttpURLConnection
        connection.requestMethod = method
        connection.setRequestProperty("X-Token", pairing.token)
        connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
        connection.connectTimeout = 4000
        connection.readTimeout = 6000
        return connection
    }

    data class Result(val rev: Long, val text: String) {
        val ok: Boolean get() = rev >= 0
    }
}

/** Clipboard access that reports whether the system let us through. */
class PhoneClipboard(private val context: Context) {
    private val manager = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    fun read(): String? = try {
        manager.primaryClip?.getItemAt(0)?.coerceToText(context)?.toString()?.takeIf { it.isNotEmpty() }
    } catch (e: SecurityException) {
        null
    }

    fun write(text: String): Boolean = try {
        manager.setPrimaryClip(ClipData.newPlainText("ClipSync", text))
        true
    } catch (e: SecurityException) {
        false
    }
}
