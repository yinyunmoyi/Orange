package com.example.orange.data.audiosync

import android.content.Context
import android.media.MediaPlayer
import com.example.orange.data.logging.AppLog as Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

object CachedAudioPlayer {
    suspend fun play(
        context: Context,
        player: MediaPlayer,
        url: String,
        logTag: String,
    ) {
        if (url.isBlank()) return
        val localPath = AudioCacheRepository.resolve(context.applicationContext, url)
        withContext(Dispatchers.Main.immediate) {
            runCatching {
                if (player.isPlaying) player.stop()
                player.reset()
                player.setDataSource(localPath ?: url)
                player.setOnPreparedListener { it.start() }
                player.setOnErrorListener { _, what, extra ->
                    Log.e(logTag, "play failed what=$what extra=$extra url=$url")
                    true
                }
                player.prepareAsync()
            }.onFailure {
                Log.e(logTag, "setup failed url=$url local=${localPath != null}", it)
            }
        }
    }
}
