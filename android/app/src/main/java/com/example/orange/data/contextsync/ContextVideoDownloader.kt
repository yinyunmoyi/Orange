package com.example.orange.data.contextsync

import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

internal object ContextVideoDownloader {
    fun downloadVideo(
        origin: String,
        path: String,
        destination: File,
        expectedBytes: Long?,
    ): Long = download(origin, path, destination, expectedBytes) { file ->
        file.length() > 0L
    }

    fun downloadVtt(origin: String, path: String, destination: File): Long =
        download(origin, path, destination, null) { file ->
            file.inputStream().buffered().use { input ->
                val prefix = ByteArray(6)
                input.read(prefix) == prefix.size &&
                    prefix.toString(Charsets.UTF_8) == "WEBVTT"
            }
        }

    private fun download(
        origin: String,
        path: String,
        destination: File,
        expectedBytes: Long?,
        validator: (File) -> Boolean,
    ): Long {
        require(path.isSafeApiPath())
        destination.parentFile?.mkdirs()
        destination.delete()
        val connection = URL(origin + path).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.connectTimeout = 5_000
            connection.readTimeout = 60_000
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Connection", "close")
            val code = connection.responseCode
            if (code !in 200..299) throw IOException("HTTP $code")
            val contentLength = connection.contentLengthLong
            if (expectedBytes != null && contentLength > 0L && contentLength != expectedBytes) {
                throw IOException("unexpected content length")
            }
            connection.inputStream.use { input ->
                destination.outputStream().use { output ->
                    input.copyTo(output)
                    output.fd.sync()
                }
            }
            val actual = destination.length()
            if (expectedBytes != null && actual != expectedBytes) {
                throw IOException("unexpected file size")
            }
            if (!validator(destination)) throw IOException("invalid downloaded file")
            return actual
        } catch (error: Throwable) {
            destination.delete()
            throw error
        } finally {
            connection.disconnect()
        }
    }
}
