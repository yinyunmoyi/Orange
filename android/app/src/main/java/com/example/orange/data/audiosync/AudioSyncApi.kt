package com.example.orange.data.audiosync

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

object AudioSyncApi {
    suspend fun manifest(origin: String, serverId: String): Result<AudioSyncManifest> =
        withContext(Dispatchers.IO) {
            runCatching {
                parseAudioSyncManifest(
                    getJson("$origin/api/v1/audio-sync/manifest"),
                    serverId,
                )
            }
        }

    suspend fun download(
        urlString: String,
        target: File,
        expectedBytes: Long? = null,
    ): Long = withContext(Dispatchers.IO) {
        val connection = URL(urlString).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.connectTimeout = 5_000
            connection.readTimeout = 30_000
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "audio/mpeg")
            val code = connection.responseCode
            if (code !in 200..299) throw IOException("HTTP $code")
            val contentLength = connection.contentLengthLong.takeIf { it >= 0L }
            if (expectedBytes != null && contentLength != null && contentLength != expectedBytes) {
                throw IOException("audio content length mismatch")
            }
            target.parentFile?.mkdirs()
            var total = 0L
            target.outputStream().use { output ->
                connection.inputStream.use { input ->
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    while (true) {
                        val count = input.read(buffer)
                        if (count < 0) break
                        output.write(buffer, 0, count)
                        total += count
                    }
                }
                output.fd.sync()
            }
            if ((expectedBytes != null && total != expectedBytes) ||
                (contentLength != null && total != contentLength)
            ) {
                throw IOException("audio size mismatch")
            }
            if (total <= 0L) throw IOException("empty audio")
            total
        } finally {
            connection.disconnect()
        }
    }

    private fun getJson(urlString: String): JSONObject {
        val connection = URL(urlString).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.connectTimeout = 3_000
            connection.readTimeout = 20_000
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json")
            val code = connection.responseCode
            val body = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) throw IOException("HTTP $code")
            return JSONObject(body)
        } finally {
            connection.disconnect()
        }
    }
}
