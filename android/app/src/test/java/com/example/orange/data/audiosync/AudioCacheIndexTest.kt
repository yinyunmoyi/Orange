package com.example.orange.data.audiosync

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files

class AudioCacheIndexTest {
    @Test
    fun roundTripsIndexAtomically() {
        val root = Files.createTempDirectory("audio-index").toFile()
        val expected = AudioCacheIndex(
            serverId = "server",
            entries = listOf(
                CachedAudio(
                    itemType = "word",
                    itemId = 7,
                    audioId = 3,
                    accent = "US",
                    version = "version-one",
                    playbackUrl = "/api/v1/word/audio?src=test",
                    fileName = "word-3-version-one.mp3",
                    actualBytes = 9000,
                ),
            ),
        )

        AudioCacheIndexStore.write(root, expected)

        assertEquals(expected, AudioCacheIndexStore.read(root))
        assertTrue(!root.resolve("index.json.tmp").exists())
    }

    @Test
    fun corruptIndexFallsBackToEmpty() {
        val root = Files.createTempDirectory("audio-index-corrupt").toFile()
        root.resolve("index.json").writeText("{bad")

        assertEquals(AudioCacheIndex("", emptyList()), AudioCacheIndexStore.read(root))
    }
}
