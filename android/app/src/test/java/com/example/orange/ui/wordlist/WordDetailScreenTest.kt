package com.example.orange.ui.wordlist

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.SpanStyle
import com.example.orange.data.word.ContextVideo
import com.example.orange.data.word.FavoriteStatus
import com.example.orange.data.word.sanitizeContextVideos
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class WordDetailScreenTest {

    @Test
    fun sanitizeContextVideos_filtersInvalidRecordsAndKeepsMultipleOccurrences() {
        val videos = sanitizeContextVideos(
            listOf(
                ContextVideo(1, "/video/1", "/subtitle/1", 9_000),
                ContextVideo(2, "/video/2", "", 8_000),
                ContextVideo(0, "/video/invalid", "", 8_000),
                ContextVideo(3, "", "", 8_000),
                ContextVideo(4, "/video/4", "", 0),
            ),
        )

        assertEquals(listOf(1L, 2L), videos.map { it.id })
    }

    @Test
    fun favoriteItemIdForDelete_onlyReturnsConfirmedFavoriteId() {
        assertNull(favoriteItemIdForDelete(null))
        assertNull(favoriteItemIdForDelete(FavoriteStatus(favorited = false, itemId = 7)))
        assertNull(favoriteItemIdForDelete(FavoriteStatus(favorited = true, itemId = 0)))
        assertEquals(9L, favoriteItemIdForDelete(FavoriteStatus(favorited = true, itemId = 9)))
        assertEquals(5, FavoriteStatus(favorited = true, itemId = 9, level = 5).level)
    }

    @Test
    fun favoriteItemIdForReset_onlyReturnsConfirmedFavoriteId() {
        assertNull(favoriteItemIdForReset(null))
        assertNull(favoriteItemIdForReset(FavoriteStatus(favorited = false, itemId = 7)))
        assertNull(favoriteItemIdForReset(FavoriteStatus(favorited = true, itemId = 0)))

        val saved = FavoriteStatus(favorited = true, itemId = 9, level = 1)
        assertEquals(9L, favoriteItemIdForReset(saved))
        assertEquals(9L, favoriteItemIdForDelete(saved))
    }

    @Test
    fun buildHighlightedSentence_appliesExactRange() {
        val result = buildHighlightedSentence(
            sentence = "Hello world!",
            highlightStart = 6,
            highlightEnd = 11,
            highlightStyle = SpanStyle(color = Color.Red),
        )

        assertEquals("Hello world!", result.text)
        assertEquals(1, result.spanStyles.size)
        assertEquals(6, result.spanStyles.single().start)
        assertEquals(11, result.spanStyles.single().end)
        assertEquals(Color.Red, result.spanStyles.single().item.color)
    }

    @Test
    fun buildHighlightedSentence_invalidRangeFallsBackToPlainText() {
        val result = buildHighlightedSentence(
            sentence = "Hello world!",
            highlightStart = 6,
            highlightEnd = 99,
            highlightStyle = SpanStyle(color = Color.Red),
        )

        assertEquals("Hello world!", result.text)
        assertTrue(result.spanStyles.isEmpty())
    }

    @Test
    fun buildHighlightedSentence_highlightsWholePhrase() {
        val result = buildHighlightedSentence(
            sentence = "Please look up this word.",
            highlightStart = 7,
            highlightEnd = 14,
            highlightStyle = SpanStyle(color = Color.Red),
        )

        assertEquals("look up", result.text.substring(7, 14))
        assertEquals(7, result.spanStyles.single().start)
        assertEquals(14, result.spanStyles.single().end)
    }
}
