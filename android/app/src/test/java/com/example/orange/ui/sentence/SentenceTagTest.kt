package com.example.orange.ui.sentence

import com.example.orange.data.word.buildSentenceFavoritesUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SentenceTagTest {

    @Test
    fun parseSentenceTagArgb_acceptsHexAndRejectsInvalidValue() {
        assertEquals(0xFFE65100L, parseSentenceTagArgb("#E65100"))
        assertEquals(0xFFE65100L, parseSentenceTagArgb("#e65100"))
        assertNull(parseSentenceTagArgb("E65100"))
        assertNull(parseSentenceTagArgb("#12345"))
    }

    @Test
    fun toggleSentenceTagSelection_addsAndRemovesOneTag() {
        val selected = toggleSentenceTagSelection(setOf(1L), 2L)
        assertEquals(setOf(1L, 2L), selected)
        assertEquals(setOf(1L), toggleSentenceTagSelection(selected, 2L))
        assertEquals(selected, toggleSentenceTagSelection(selected, 0L))
    }

    @Test
    fun buildSentenceFavoritesUrl_sortsSelectedTagIds() {
        val url = buildSentenceFavoritesUrl(setOf(9L, 2L, 5L))
        assertTrue(url.endsWith("/api/v1/sentences/favorites?tagIds=2,5,9"))
        assertTrue(buildSentenceFavoritesUrl(emptySet()).endsWith("/api/v1/sentences/favorites"))
    }

    @Test
    fun buildSentenceTagUpdate_requiresOneSentenceNoteAndSortsTagIds() {
        val update = buildSentenceTagUpdate(
            selectedTagIds = setOf(9L, 2L),
            note = "  Shared sentence note. ",
        )

        assertEquals(listOf(2L, 9L), update!!.tagIds.toList())
        assertEquals("Shared sentence note.", update.note)
        assertNull(buildSentenceTagUpdate(setOf(2L), " "))
        assertNull(buildSentenceTagUpdate(setOf(2L), "tab\tcharacter"))
    }

    @Test
    fun buildSentenceTagUpdate_normalizesMultilineNote() {
        val update = buildSentenceTagUpdate(
            selectedTagIds = setOf(2L),
            note = " First line.\r\nSecond line. ",
        )

        assertEquals("First line.\nSecond line.", update!!.note)
    }

    @Test
    fun buildSentenceTagUpdate_allowsStandaloneNoteAndClearingAll() {
        assertEquals(
            SentenceTagUpdate(emptySet(), "Standalone note."),
            buildSentenceTagUpdate(emptySet(), " Standalone note. "),
        )
        assertEquals(
            SentenceTagUpdate(emptySet(), ""),
            buildSentenceTagUpdate(emptySet(), " "),
        )
    }
}
