package com.example.orange.data.logging

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.os.Build
import android.os.SystemClock
import com.example.orange.data.contextsync.ContextSyncPreferences
import com.example.orange.data.word.WordServiceConfig
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.util.UUID
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

object AppLog {
    private const val LOG_DIRECTORY = "logs"
    private const val PENDING_FILE = "pending-android.ndjson"
    private const val UPLOAD_PATH = "/api/v1/logs/android"
    private const val MAX_EVENT_CHARS = 16_000
    private const val MAX_QUEUE_BYTES = 2L * 1024 * 1024
    private const val MAX_BATCH_EVENTS = 64
    private const val MAX_BATCH_BYTES = 256 * 1024
    private const val RETRY_INTERVAL_MS = 10_000L

    @Volatile
    private var applicationContext: Context? = null
    private val executor = Executors.newSingleThreadExecutor { task ->
        Thread(task, "orange-log-uploader").apply { isDaemon = true }
    }
    private val initialized = AtomicBoolean(false)
    private val fileLock = Any()
    private val appSessionId = UUID.randomUUID().toString()
    private var nextRetryAt = 0L

    fun init(context: Context) {
        if (!initialized.compareAndSet(false, true)) return
        applicationContext = context.applicationContext
        installCrashHandler()
        registerNetworkCallback()
        i(
            "Application",
            "app started",
            JSONObject()
                .put("sdk", Build.VERSION.SDK_INT)
                .put("device", "${Build.MANUFACTURER} ${Build.MODEL}"),
        )
        flush()
    }

    fun d(tag: String, message: String, data: JSONObject? = null, traceId: String? = null): Int {
        val result = android.util.Log.d(tag, message)
        enqueue("DEBUG", tag, message, data, traceId, null)
        return result
    }

    fun i(tag: String, message: String, data: JSONObject? = null, traceId: String? = null): Int {
        val result = android.util.Log.i(tag, message)
        enqueue("INFO", tag, message, data, traceId, null)
        return result
    }

    fun w(tag: String, message: String, error: Throwable? = null): Int {
        val result = if (error == null) {
            android.util.Log.w(tag, message)
        } else {
            android.util.Log.w(tag, message, error)
        }
        enqueue("WARN", tag, message, null, null, error)
        return result
    }

    fun e(tag: String, message: String, error: Throwable? = null): Int {
        val result = if (error == null) {
            android.util.Log.e(tag, message)
        } else {
            android.util.Log.e(tag, message, error)
        }
        enqueue("ERROR", tag, message, null, null, error)
        return result
    }

    fun event(
        tag: String,
        message: String,
        data: JSONObject = JSONObject(),
        traceId: String? = null,
    ) {
        enqueue("INFO", tag, message, data, traceId, null)
    }

    fun flush() {
        executor.execute {
            runCatching { flushPending(force = true) }
                .onFailure { android.util.Log.w("AppLog", "flush failed", it) }
        }
    }

    private fun enqueue(
        level: String,
        tag: String,
        message: String,
        data: JSONObject?,
        traceId: String?,
        error: Throwable?,
    ) {
        val context = applicationContext ?: return
        val event = buildEvent(level, tag, message, data, traceId, error)
        executor.execute {
            runCatching {
                appendEvent(context, event)
                flushPending(force = false)
            }.onFailure {
                android.util.Log.w("AppLog", "persist or upload failed", it)
            }
        }
    }

    private fun buildEvent(
        level: String,
        tag: String,
        message: String,
        data: JSONObject?,
        traceId: String?,
        error: Throwable?,
    ): JSONObject = JSONObject()
        .put("ts", System.currentTimeMillis())
        .put("elapsedRealtimeMs", SystemClock.elapsedRealtime())
        .put("appSessionId", appSessionId)
        .put("level", level)
        .put("tag", tag.take(100))
        .put("message", message.take(MAX_EVENT_CHARS))
        .put("thread", Thread.currentThread().name.take(100))
        .also { event ->
            if (data != null) event.put("data", data)
            if (traceId != null) event.put("traceId", traceId)
            if (error != null) {
                event.put("errorType", error.javaClass.name)
                event.put(
                    "stackTrace",
                    android.util.Log.getStackTraceString(error).take(MAX_EVENT_CHARS),
                )
            }
        }

    private fun appendEvent(context: Context, event: JSONObject) {
        synchronized(fileLock) {
            val file = pendingFile(context)
            file.parentFile?.mkdirs()
            var serialized = event.toString()
            if (serialized.length > MAX_EVENT_CHARS * 2) {
                event.remove("data")
                event.put("dataTruncated", true)
                serialized = event.toString()
            }
            file.appendText(serialized + "\n", Charsets.UTF_8)
            if (file.length() > MAX_QUEUE_BYTES) {
                val lines = file.readLines(Charsets.UTF_8)
                file.writeText(lines.takeLast(lines.size / 2).joinToString("\n", postfix = "\n"))
            }
        }
    }

    private fun flushPending(force: Boolean) {
        val context = applicationContext ?: return
        val now = SystemClock.elapsedRealtime()
        if (!force && now < nextRetryAt) return
        repeat(10) {
            val batch = readBatch(context)
            if (batch.isEmpty()) return
            if (!upload(context, batch)) {
                nextRetryAt = SystemClock.elapsedRealtime() + RETRY_INTERVAL_MS
                return
            }
            acknowledge(context, batch.size)
            nextRetryAt = 0L
        }
    }

    private fun readBatch(context: Context): List<String> = synchronized(fileLock) {
        val file = pendingFile(context)
        if (!file.exists()) return@synchronized emptyList()
        val result = mutableListOf<String>()
        var bytes = 0
        file.useLines(Charsets.UTF_8) { lines ->
            for (line in lines) {
                val lineBytes = line.toByteArray(Charsets.UTF_8).size
                if (result.isNotEmpty() && bytes + lineBytes > MAX_BATCH_BYTES) break
                result += line
                bytes += lineBytes
                if (result.size >= MAX_BATCH_EVENTS) break
            }
        }
        result
    }

    private fun acknowledge(context: Context, count: Int) {
        synchronized(fileLock) {
            val file = pendingFile(context)
            if (!file.exists()) return
            val remaining = file.readLines(Charsets.UTF_8).drop(count)
            if (remaining.isEmpty()) {
                file.writeText("")
            } else {
                file.writeText(remaining.joinToString("\n", postfix = "\n"))
            }
        }
    }

    private fun upload(context: Context, lines: List<String>): Boolean {
        val events = JSONArray()
        lines.forEach { line ->
            events.put(runCatching { JSONObject(line) }.getOrElse { JSONObject().put("raw", line) })
        }
        val payload = JSONObject()
            .put("appSessionId", appSessionId)
            .put("events", events)
            .toString()
            .toByteArray(Charsets.UTF_8)
        val localOrigin = ContextSyncPreferences(context).localOrigin?.trimEnd('/')
        val origins = listOfNotNull(localOrigin, WordServiceConfig.origin).distinct()
        for (origin in origins) {
            val connection = runCatching {
                URL(origin + UPLOAD_PATH).openConnection() as HttpURLConnection
            }.getOrNull() ?: continue
            val delivered = try {
                connection.requestMethod = "POST"
                connection.connectTimeout = if (origin == localOrigin) 700 else 3_000
                connection.readTimeout = 3_000
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                connection.setRequestProperty("Connection", "close")
                connection.outputStream.use { it.write(payload) }
                connection.responseCode in 200..299
            } catch (_: Throwable) {
                false
            } finally {
                connection.disconnect()
            }
            if (delivered) return true
        }
        return false
    }

    private fun installCrashHandler() {
        val previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, error ->
            applicationContext?.let { context ->
                runCatching {
                    appendEvent(
                        context,
                        buildEvent(
                            level = "FATAL",
                            tag = "UncaughtException",
                            message = "uncaught exception on ${thread.name}",
                            data = null,
                            traceId = null,
                            error = error,
                        ),
                    )
                }
            }
            previous?.uncaughtException(thread, error)
        }
    }

    private fun registerNetworkCallback() {
        val context = applicationContext ?: return
        val connectivity = context.getSystemService(ConnectivityManager::class.java)
        runCatching {
            connectivity.registerDefaultNetworkCallback(
                object : ConnectivityManager.NetworkCallback() {
                    override fun onAvailable(network: Network) {
                        flush()
                    }
                },
            )
        }
    }

    private fun pendingFile(context: Context): File =
        File(File(context.filesDir, LOG_DIRECTORY), PENDING_FILE)
}
