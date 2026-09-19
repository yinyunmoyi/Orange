package com.example.orange.ui.sentence

import com.example.orange.data.word.SentenceFavorite
import com.example.orange.data.word.sanitizeSentenceFavorites
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SentenceListScreenTest {

    @Test
    fun sanitizeSentenceFavorites_filtersInvalidItemsAndKeepsOrder() {
        val newest = SentenceFavorite(3L, "Newest sentence.", "Newest translation.", "2026-08-22T12:00:00Z")
        val invalidId = SentenceFavorite(0L, "Invalid id.", "Translation.", "2026-08-22T11:00:00Z")
        val blankSentence = SentenceFavorite(2L, " ", "Translation.", "2026-08-22T10:00:00Z")
        val oldest = SentenceFavorite(1L, "Oldest sentence.", "Oldest translation.", "2026-08-22T09:00:00Z")

        val result = sanitizeSentenceFavorites(
            listOf(newest, invalidId, blankSentence, oldest),
        )

        assertEquals(listOf(3L, 1L), result.map { it.id })
    }

    @Test
    fun sentenceFavoriteIdForOpen_acceptsOnlyValidItems() {
        assertEquals(
            7L,
            sentenceFavoriteIdForOpen(
                SentenceFavorite(7L, "Valid sentence.", "Translation.", ""),
            ),
        )
        assertNull(
            sentenceFavoriteIdForOpen(
                SentenceFavorite(0L, "Invalid sentence.", "Translation.", ""),
            ),
        )
        assertNull(
            sentenceFavoriteIdForOpen(
                SentenceFavorite(7L, " ", "Translation.", ""),
            ),
        )
    }
}
