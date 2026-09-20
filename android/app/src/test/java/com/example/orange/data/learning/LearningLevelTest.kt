package com.example.orange.data.learning

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Test

class LearningLevelTest {
    @Test
    fun parseCurrentReadsLevelAndDefaultsMissingLevel() {
        val withLevel = response(level = 7)
        val withoutLevel = response(level = null)

        assertEquals(7, LearningApi.parseCurrent(withLevel).card?.level)
        assertEquals(0, LearningApi.parseCurrent(withoutLevel).card?.level)
    }

    private fun response(level: Int?): JSONObject {
        val card = JSONObject()
            .put("itemType", LEARNING_ITEM_WORD)
            .put("itemId", 8)
            .put("word", "example")
        if (level != null) card.put("level", level)
        return JSONObject().put(
            "data",
            JSONObject()
                .put("sessionId", 1)
                .put("turnNo", 0)
                .put("queueItemId", 2)
                .put("remaining", 1)
                .put("completed", false)
                .put("card", card),
        )
    }
}
