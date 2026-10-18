package com.example.orange.data.audiosync

import android.content.Context
import android.media.MediaPlayer
import android.os.SystemClock
import com.example.orange.data.contextsync.ContextSyncPreferences
import com.example.orange.data.logging.AppLog as Log
import com.example.orange.data.network.NetworkAccessPolicy
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.net.URL

object CachedAudioPlayer {
    suspend fun play(
        context: Context,
        player: MediaPlayer,
        url: String,
        logTag: String,
    ) {
        if (url.isBlank()) return
        val appContext = context.applicationContext
        val localPath = AudioCacheRepository.resolve(appContext, url)
        val sources = if (localPath != null) {
            listOf(AudioPlaybackSource(localPath, "cache"))
        } else {
            val localOrigin = ContextSyncPreferences(appContext).localOrigin
            val candidates = audioPlaybackCandidates(url, localOrigin)
            val localCandidate = candidates.firstOrNull { it.kind == "lan" }
            if (localCandidate != null && probeLocalAudio(localCandidate.url, logTag)) {
                candidates
            } else {
                candidates.filterNot { it.kind == "lan" }
            }
        }
        if (sources.isEmpty()) return
        withContext(Dispatchers.Main.immediate) {
            if (player.isPlaying) player.stop()
            prepare(player, sources, 0, logTag)
        }
    }

    private fun prepare(
        player: MediaPlayer,
        sources: List<AudioPlaybackSource>,
        index: Int,
        logTag: String,
    ) {
        val source = sources.getOrNull(index) ?: return
        val started = SystemClock.elapsedRealtime()
        runCatching {
            player.reset()
            player.setDataSource(source.url)
            player.setOnPreparedListener {
                Log.i(
                    logTag,
                    "prepared source=${source.kind} duration_ms=${SystemClock.elapsedRealtime() - started}",
                )
                it.start()
            }
            player.setOnErrorListener { _, what, extra ->
                val hasFallback = index + 1 < sources.size
                Log.w(
                    logTag,
                    "play failed source=${source.kind} what=$what extra=$extra " +
                        "duration_ms=${SystemClock.elapsedRealtime() - started} fallback=$hasFallback",
                )
                if (hasFallback) {
                    prepare(player, sources, index + 1, logTag)
                }
                true
            }
            player.prepareAsync()
        }.onFailure {
            Log.w(
                logTag,
                "setup failed source=${source.kind} duration_ms=${SystemClock.elapsedRealtime() - started}",
                it,
            )
            if (index + 1 < sources.size) {
                prepare(player, sources, index + 1, logTag)
            }
        }
    }

    private suspend fun probeLocalAudio(url: String, logTag: String): Boolean =
        withContext(Dispatchers.IO) {
            val started = SystemClock.elapsedRealtime()
            val connection = runCatching {
                NetworkAccessPolicy.openConnection(URL(url))
            }.getOrElse {
                Log.w(logTag, "LAN audio probe setup failed", it)
                return@withContext false
            }
            try {
                connection.requestMethod = "GET"
                connection.connectTimeout = LAN_CONNECT_TIMEOUT_MS
                connection.readTimeout = LAN_READ_TIMEOUT_MS
                connection.instanceFollowRedirects = false
                connection.setRequestProperty("Range", "bytes=0-0")
                val success = connection.responseCode in 200..299
                (if (success) connection.inputStream else connection.errorStream)?.close()
                Log.i(
                    logTag,
                    "LAN audio probe success=$success code=${connection.responseCode} " +
                        "duration_ms=${SystemClock.elapsedRealtime() - started}",
                )
                success
            } catch (error: Throwable) {
                Log.w(
                    logTag,
                    "LAN audio probe failed duration_ms=${SystemClock.elapsedRealtime() - started}",
                    error,
                )
                false
            } finally {
                NetworkAccessPolicy.release(connection)
            }
        }

    private const val LAN_CONNECT_TIMEOUT_MS = 700
    private const val LAN_READ_TIMEOUT_MS = 1_000
}

internal data class AudioPlaybackSource(
    val url: String,
    val kind: String,
)

internal fun audioPlaybackCandidates(
    publicUrl: String,
    localOrigin: String?,
): List<AudioPlaybackSource> {
    val localUrl = localOrigin
        ?.trim()
        ?.trimEnd('/')
        ?.takeIf { it.startsWith("http://") || it.startsWith("https://") }
        ?.let { origin ->
            AudioCacheRepository.normalizePlaybackUrl(publicUrl)?.let { path -> origin + path }
        }
    return buildList {
        if (localUrl != null && localUrl != publicUrl) {
            add(AudioPlaybackSource(localUrl, "lan"))
        }
        add(AudioPlaybackSource(publicUrl, "public"))
    }
}
