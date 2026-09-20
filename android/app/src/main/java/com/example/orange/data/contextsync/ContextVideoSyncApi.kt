package com.example.orange.data.contextsync

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

object ContextVideoSyncApi {
    suspend fun ping(origin: String): Result<String> = withContext(Dispatchers.IO) {
        runCatching {
            val root = getJson("$origin/api/v1/context-video-sync/ping", 1_500, 1_500)
            val data = root.optJSONObject("data")
                ?: throw IOException(root.optString("msg", "response missing data"))
            check(data.optInt("apiVersion") == 1) { "unsupported sync api" }
            data.optString("serverId").trim().ifEmpty {
                throw IOException("response missing serverId")
            }
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
    ): JSONObject {
        val connection = URL(urlString).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.connectTimeout = connectTimeout
            connection.readTimeout = readTimeout
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            val code = connection.responseCode
            val body = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                throw IOException("HTTP $code")
            }
            return JSONObject(body)
        } finally {
            connection.disconnect()
        }
    }
}
