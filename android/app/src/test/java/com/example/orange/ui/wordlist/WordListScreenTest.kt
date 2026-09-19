package com.example.orange.ui.wordlist

import com.example.orange.data.word.FavoriteGroupSummary
import com.example.orange.data.word.FavoriteItem
import com.example.orange.data.word.FavoritePageData
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class WordListScreenTest {

    @Test
    fun normalizeWordSearchTarget_acceptsAndNormalizesSingleWords() {
        assertEquals("water", normalizeWordSearchTarget("  WATER  "))
        assertEquals("cutting-edge", normalizeWordSearchTarget("Cutting-Edge"))
        assertEquals("don't", normalizeWordSearchTarget("Don't"))
    }

    @Test
    fun normalizeWordSearchTarget_rejectsPhrasesAndInvalidInput() {
        assertNull(normalizeWordSearchTarget(""))
        assertNull(normalizeWordSearchTarget("look up"))
        assertNull(normalizeWordSearchTarget("word!"))
        assertNull(normalizeWordSearchTarget("-word"))
    }

    @Test
    fun favoriteGroupTitle_formatsDayWeekAndMonth() {
        val base = FavoriteGroupSummary(
            id = "group",
            type = "day",
            rangeStart = "2026-08-10T00:00:00+08:00",
            rangeEnd = "2026-08-11T00:00:00+08:00",
            displayStart = "2026-08-10T00:00:00+08:00",
            displayEnd = "2026-08-10T23:59:59+08:00",
            count = 2,
        )

        assertEquals("8月10日", favoriteGroupTitle(base))
        assertEquals(
            "7月27日 - 8月2日",
            favoriteGroupTitle(
                base.copy(
                    type = "week",
                    displayStart = "2026-07-27T00:00:00+08:00",
                    displayEnd = "2026-08-02T23:59:59+08:00",
                ),
            ),
        )
        assertEquals(
            "2026年7月",
            favoriteGroupTitle(base.copy(type = "month", displayStart = "2026-07-01T00:00:00+08:00")),
        )
    }

    @Test
    fun applyFavoritePage_keepsWordAndPhraseWithSameIdAndDeduplicatesRetry() {
        val summary = FavoriteGroupSummary(
            id = "day-2026-08-10",
            type = "day",
            rangeStart = "2026-08-10T00:00:00+08:00",
            rangeEnd = "2026-08-11T00:00:00+08:00",
            displayStart = "2026-08-10T00:00:00+08:00",
            displayEnd = "2026-08-10T00:00:00+08:00",
            count = 2,
        )
        val word = FavoriteItem("word", 1L, "look", "", "v.", "看", level = 4)
        val phrase = FavoriteItem("phrase", 1L, "look up", "", "短语动词", "查找")
        val state = FavoriteGroupState(summary = summary, items = listOf(word), loading = true)

        val result = applyFavoritePage(
            state,
            FavoritePageData(items = listOf(word, phrase), nextCursor = "next"),
        )

        assertEquals(listOf("word", "phrase"), result.items.map { it.itemType })
        assertEquals(listOf(4, 0), result.items.map { it.level })
        assertEquals("next", result.nextCursor)
        assertEquals(true, result.loadedOnce)
    }
}
