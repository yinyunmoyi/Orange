package com.example.orange.data.contextsync

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ContextVideoSyncModelsTest {
    @Test
    fun parseManifest_filtersInvalidAndAbsoluteUrls() {
        val valid = JSONObject()
            .put("videoId", 1)
            .put("contextId", 2)
            .put("itemType", "word")
            .put("itemId", 3)
            .put("durationMs", 4_000)
            .put("fileSize", 100)
            .put("version", "version")
            .put("videoUrl", "/api/v1/context-videos/1/video")
            .put("subtitleUrl", "/api/v1/context-videos/1/subtitles.vtt")
            .put("priority", 0)
        val absolute = JSONObject(valid.toString())
            .put("videoId", 2)
            .put("videoUrl", "https://example.com/video")
        val root = JSONObject().put(
            "data",
            JSONObject()
                .put("serverId", "server")
                .put("items", JSONArray().put(valid).put(absolute)),
        )

        val manifest = parseContextVideoSyncManifest(root, "server")

        assertEquals(listOf(1L), manifest.items.map { it.videoId })
    }

    @Test
    fun safeApiPath_rejectsTraversalAndProtocolRelativeUrls() {
        assertTrue("/api/v1/context-videos/1/video".isSafeApiPath())
        assertEquals(false, "//host/api/v1/video".isSafeApiPath())
        assertEquals(false, "/api/v1/../secret".isSafeApiPath())
    }
}
