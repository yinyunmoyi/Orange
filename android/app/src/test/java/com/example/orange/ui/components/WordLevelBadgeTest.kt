package com.example.orange.ui.components

import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertEquals
import org.junit.Test

class WordLevelBadgeTest {

    @Test
    fun wordLevelBadgeColors_mapsLevelRangesToExpectedBackgrounds() {
        val expectedColors = mapOf(
            1 to Color(0xFF2E7D32),
            9 to Color(0xFF2E7D32),
            10 to Color(0xFF1565C0),
            19 to Color(0xFF1565C0),
            20 to Color(0xFFF9A825),
            29 to Color(0xFFF9A825),
            30 to Color(0xFFC62828),
            39 to Color(0xFFC62828),
            40 to Color.Black,
            100 to Color.Black,
        )

        expectedColors.forEach { (level, expected) ->
            assertEquals(expected, wordLevelBadgeColors(level).background)
        }
    }

    @Test
    fun wordLevelBadgeColors_usesReadableContentColors() {
        assertEquals(Color.White, wordLevelBadgeColors(1).content)
        assertEquals(Color.White, wordLevelBadgeColors(10).content)
        assertEquals(Color(0xFF1C1B1F), wordLevelBadgeColors(20).content)
        assertEquals(Color.White, wordLevelBadgeColors(30).content)
        assertEquals(Color.White, wordLevelBadgeColors(40).content)
    }
}
