package com.example.orange.ui.note

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NoteEditScreenTest {

    @Test
    fun noteCodePointCount_countsEmojiAsOneCharacter() {
        assertEquals(3, noteCodePointCount("A😀中"))
    }

    @Test
    fun canSaveNote_allowsLimitAndBlankClear() {
        assertTrue(canSaveNote("好".repeat(MAX_NOTE_CODE_POINTS), saving = false))
        assertTrue(canSaveNote("   ", saving = false))
    }

    @Test
    fun canSaveNote_rejectsOverLimitAndSavingState() {
        assertFalse(canSaveNote("好".repeat(MAX_NOTE_CODE_POINTS + 1), saving = false))
        assertFalse(canSaveNote("valid", saving = true))
    }
}
