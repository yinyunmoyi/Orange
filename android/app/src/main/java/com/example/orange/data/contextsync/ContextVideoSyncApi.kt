package com.example.orange.data.contextsync

import android.os.SystemClock
import com.example.orange.data.logging.AppLog
import com.example.orange.data.network.NetworkAccessPolicy
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.IOException
import java.net.ConnectException
import java.net.HttpURLConnection
import java.net.NoRouteToHostException
import java.net.SocketTimeoutException
import java.net.URL

sealed interface ForwardProbeResult {
    data class Verified(val serverId: String) : ForwardProbeResult
    data class ConnectFailed(val phase: String, val cause: Throwable) : ForwardProbeResult
    data class HttpFailed(val status: Int) : ForwardProbeResult
    data class IdentityRejected(val actualServerId: String?) : ForwardProbeResult
}

object ContextVideoSyncApi {
    suspend fun ping(origin: String): Result<String> = withContext(Dispatchers.IO) {
        when (val result = probe(origin, expectedServerId = null)) {
            is ForwardProbeResult.Verified -> Result.success(result.serverId)
            is ForwardProbeResult.ConnectFailed -> Result.failure(result.cause)
            is ForwardProbeResult.HttpFailed ->
                Result.failure(IOException("HTTP ${result.status}"))
            is ForwardProbeResult.IdentityRejected ->
                Result.failure(IOException("context sync identity rejected"))
        }
    }

    suspend fun probe(
        origin: String,
        expectedServerId: String?,
    ): ForwardProbeResult = withContext(Dispatchers.IO) {
        val started = SystemClock.elapsedRealtime()
        var phase = "open_connection"
        var connection: HttpURLConnection? = null
        try {
            connection = NetworkAccessPolicy.openConnection(
                URL("$origin/api/v1/context-video-sync/ping"),
            )
            connection.requestMethod = "GET"
            connection.connectTimeout = 1_500
            connection.readTimeout = 1_500
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            phase = "connect"
            connection.connect()
            phase = "response_headers"
            val status = connection.responseCode
            phase = "response_body"
            val body = if (status in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (status !in 200..299) return@withContext ForwardProbeResult.HttpFailed(status)
            phase = "validate_response"
            val root = runCatching { JSONObject(body) }.getOrNull()
                ?: return@withContext ForwardProbeResult.IdentityRejected(null)
            phase = "validate_response"
            val data = root.optJSONObject("data")
                ?: return@withContext ForwardProbeResult.IdentityRejected(null)
            if (data.optInt("apiVersion") != 1) {
                return@withContext ForwardProbeResult.IdentityRejected(null)
            }
            val actualServerId = data.optString("serverId").trim().takeIf(String::isNotEmpty)
            if (
                actualServerId == null ||
                (expectedServerId != null && actualServerId != expectedServerId)
            ) {
                return@withContext ForwardProbeResult.IdentityRejected(actualServerId)
            }
            ForwardProbeResult.Verified(actualServerId)
        } catch (error: Throwable) {
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
            if (
                error is ConnectException ||
                error is NoRouteToHostException ||
                (error is SocketTimeoutException && phase == "connect")
            ) {
                ForwardProbeResult.ConnectFailed(phase, error)
            } else {
                ForwardProbeResult.IdentityRejected(null)
            }
        } finally {
            connection?.let(NetworkAccessPolicy::release)
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
