package com.example.orange.data.contextsync

import android.os.SystemClock
import com.example.orange.data.logging.AppLog
import com.example.orange.data.network.NetworkAccessPolicy
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

object ContextVideoSyncApi {
    suspend fun ping(origin: String): Result<String> = withContext(Dispatchers.IO) {
        val started = SystemClock.elapsedRealtime()
        var phase = "open_connection"
        runCatching {
            val root = getJson("$origin/api/v1/context-video-sync/ping", 1_500, 1_500) {
                phase = it
            }
            phase = "validate_response"
            val data = root.optJSONObject("data")
                ?: throw IOException(root.optString("msg", "response missing data"))
            check(data.optInt("apiVersion") == 1) { "unsupported sync api" }
            data.optString("serverId").trim().ifEmpty {
                throw IOException("response missing serverId")
            }
        }.onFailure { error ->
            AppLog.w(
                tag = "LanDiscovery",
                message = "ping_failed",
                error = error,
                data = JSONObject()
                    .put("origin", origin)
                    .put("phase", phase)
                    .put("durationMs", SystemClock.elapsedRealtime() - started)
                    .put("connectTimeoutMs", 1_500)
                    .put("readTimeoutMs", 1_500)
                    .put("errorMessage", error.message.orEmpty().take(2_000)),
                immediateUpload = true,
            )
        }
    }

    suspend fun manifest(origin: String, serverId: String): Result<ContextVideoSyncManifest> =
        withContext(Dispatchers.IO) {
            runCatching {
                parseContextVideoSyncManifest(
                    getJson(
                        "$origin/api/v1/context-video-sync/manifest?days=7&limit=1000",
                        3_000,
                        15_000,
                    ),
                    serverId,
                )
            }
        }

    private fun getJson(
        urlString: String,
        connectTimeout: Int,
        readTimeout: Int,
        onPhase: (String) -> Unit = {},
    ): JSONObject {
        val connection = NetworkAccessPolicy.openConnection(URL(urlString))
        try {
            connection.requestMethod = "GET"
            connection.connectTimeout = connectTimeout
            connection.readTimeout = readTimeout
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            onPhase("connect_or_response_headers")
            val code = connection.responseCode
            onPhase("response_body")
            val body = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                throw IOException("HTTP $code")
            }
            onPhase("parse_response")
            return JSONObject(body)
        } finally {
            NetworkAccessPolicy.release(connection)
        }
    }
}
