package com.example.orange.data.word

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Test

class WordActionEventTest {
    @Test
    fun buildFavoriteActionBodyIncludesStableContract() {
        val body = buildFavoriteActionBody(
            eventId = "event-12345678",
            action = "audio_played",
            source = "android_learning",
            metadata = JSONObject().put("mode", "auto"),
        )

        assertEquals("event-12345678", body.getString("eventId"))
        assertEquals("audio_played", body.getString("action"))
        assertEquals("android_learning", body.getString("source"))
        assertEquals("auto", body.getJSONObject("metadata").getString("mode"))
    }
}
