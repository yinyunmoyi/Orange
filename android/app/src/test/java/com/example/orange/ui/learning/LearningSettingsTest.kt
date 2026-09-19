package com.example.orange.ui.learning

import com.example.orange.data.learning.parseLearningSettingsResponse
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.io.IOException

class LearningSettingsTest {

    @Test
    fun dailyNewOptions_useConfiguredRangeAndStep() {
        assertEquals(5, DAILY_NEW_OPTIONS.first())
        assertEquals(500, DAILY_NEW_OPTIONS.last())
        assertEquals(100, DAILY_NEW_OPTIONS.size)
        assertEquals(5, DAILY_NEW_OPTIONS.zipWithNext().first().second - DAILY_NEW_OPTIONS.first())
    }

    @Test
    fun dailyReviewOptions_appendUnlimitedAfterFiniteRange() {
        assertEquals(5, DAILY_REVIEW_OPTIONS.first())
        assertEquals(1000, DAILY_REVIEW_OPTIONS[DAILY_REVIEW_OPTIONS.lastIndex - 1])
        assertEquals(UNLIMITED_REVIEW_LIMIT, DAILY_REVIEW_OPTIONS.last())
        assertEquals(1, DAILY_REVIEW_OPTIONS.count { it == UNLIMITED_REVIEW_LIMIT })
    }

    @Test
    fun estimatedCompletionDate_handlesZeroExactAndRoundedDays() {
        assertEquals("2026-08-21", estimatedCompletionDate("2026-08-21", 0, 30))
        assertEquals("2026-08-22", estimatedCompletionDate("2026-08-21", 30, 30))
        assertEquals("2026-08-23", estimatedCompletionDate("2026-08-21", 31, 30))
    }

    @Test
    fun parseLearningSettingsResponse_readsValidPayload() {
        val result = parseLearningSettingsResponse(
            JSONObject(
                """
                {
                  "code": 0,
                  "data": {
                    "dailyNewLimit": 30,
                    "dailyReviewLimit": 9999,
                    "remainingNewCount": 774,
                    "studyDate": "2026-08-21"
                  }
                }
                """.trimIndent(),
            ),
        )

        assertEquals(30, result.dailyNewLimit)
        assertEquals(9999, result.dailyReviewLimit)
        assertEquals(774, result.remainingNewCount)
        assertEquals("2026-08-21", result.studyDate)
    }

    @Test
    fun parseLearningSettingsResponse_rejectsMissingOrInvalidData() {
        assertThrows(IOException::class.java) {
            parseLearningSettingsResponse(JSONObject("""{"code":0}"""))
        }
        assertThrows(IOException::class.java) {
            parseLearningSettingsResponse(
                JSONObject(
                    """
                    {
                      "data": {
                        "dailyNewLimit": 30,
                        "dailyReviewLimit": 9999,
                        "remainingNewCount": -1,
                        "studyDate": "invalid"
                      }
                    }
                    """.trimIndent(),
                ),
            )
        }
    }
}
