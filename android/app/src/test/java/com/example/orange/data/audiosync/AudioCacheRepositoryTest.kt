package com.example.orange.data.audiosync

import org.junit.Assert.assertEquals
import org.junit.Test

class AudioCacheRepositoryTest {
    @Test
    fun normalizesHostAndRegenerationVersion() {
        val relative = "/api/v1/word/audio?src=tts%3A%2F%2Fus%2Ftest"
        val absolute = "http://192.0.2.22:8888$relative&v=new-version"

        assertEquals(relative, AudioCacheRepository.normalizePlaybackUrl(absolute))
        assertEquals(relative, AudioCacheRepository.normalizePlaybackUrl(relative))
    }

    @Test
    fun invalidatingWordRemovesAllAccentsOnlyForThatWord() {
        val entries = listOf(
            cached("word", 7, 1, "US"),
            cached("word", 7, 2, "UK"),
            cached("word", 8, 3, "US"),
            cached("phrase", 7, 7, ""),
        )

        val remaining = withoutAudioItem(entries, "word", 7)

        assertEquals(listOf(entries[2], entries[3]), remaining)
    }

    private fun cached(
        type: String,
        itemId: Long,
        audioId: Long,
        accent: String,
    ) = CachedAudio(
        itemType = type,
        itemId = itemId,
        audioId = audioId,
        accent = accent,
        version = "version",
        playbackUrl = if (type == "word") {
            "/api/v1/word/audio?src=$audioId"
        } else {
            "/api/v1/phrase/$itemId/audio"
        },
        fileName = "$type-$audioId.mp3",
        actualBytes = 10,
    )
}
