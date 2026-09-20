package com.example.orange.data.audiosync

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class AudioSyncModelsTest {
    @Test
    fun parsesValidItemsAndFiltersUnsafeOnes() {
        val version = "a".repeat(64)
        val root = JSONObject(
            """
            {
              "code": 0,
              "data": {
                "serverId": "server",
                "generatedAt": "2026-08-21T12:00:00Z",
                "items": [
                  {
                    "itemType": "word",
                    "itemId": 7,
                    "audioId": 3,
                    "accent": "US",
                    "fileSize": 9000,
                    "version": "$version",
                    "audioUrl": "/api/v1/audio-sync/items/word/3?v=$version",
                    "playbackUrl": "/api/v1/word/audio?src=tts%3A%2F%2Fus%2Ftest"
                  },
                  {
                    "itemType": "phrase",
                    "itemId": 9,
                    "audioId": 0,
                    "fileSize": 9500,
                    "version": "$version",
                    "audioUrl": "https://example.com/unsafe.mp3",
                    "playbackUrl": "/api/v1/phrase/9/audio"
                  }
                ]
              }
            }
            """.trimIndent(),
        )

        val manifest = parseAudioSyncManifest(root, "server")

        assertEquals(1, manifest.items.size)
        assertEquals(3L, manifest.items.single().audioId)
    }

    @Test
    fun rejectsDifferentServer() {
        val root = JSONObject(
            """{"code":0,"data":{"serverId":"other","items":[]}}""",
        )

        assertThrows(IllegalArgumentException::class.java) {
            parseAudioSyncManifest(root, "expected")
        }
    }
}
