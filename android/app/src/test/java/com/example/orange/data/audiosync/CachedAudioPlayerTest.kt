package com.example.orange.data.audiosync

import org.junit.Assert.assertEquals
import org.junit.Test

class CachedAudioPlayerTest {

    @Test
    fun candidatesPreferLanForInternalAudioUrl() {
        val sources = audioPlaybackCandidates(
            publicUrl = "https://public.example/api/v1/phrase/1021/audio",
            localOrigin = "http://192.168.1.6:8888/",
        )

        assertEquals(
            listOf(
                AudioPlaybackSource("http://192.168.1.6:8888/api/v1/phrase/1021/audio", "lan"),
                AudioPlaybackSource("https://public.example/api/v1/phrase/1021/audio", "public"),
            ),
            sources,
        )
    }

    @Test
    fun candidatesKeepPublicFallbackForMissingLanOrigin() {
        val publicUrl = "https://public.example/api/v1/contexts/354/audio"

        assertEquals(
            listOf(AudioPlaybackSource(publicUrl, "public")),
            audioPlaybackCandidates(publicUrl, null),
        )
    }

    @Test
    fun candidatesDoNotRedirectExternalMediaToLan() {
        val publicUrl = "https://cdn.example/audio/test.mp3"

        assertEquals(
            listOf(AudioPlaybackSource(publicUrl, "public")),
            audioPlaybackCandidates(publicUrl, "http://192.168.1.6:8888"),
        )
    }
}
