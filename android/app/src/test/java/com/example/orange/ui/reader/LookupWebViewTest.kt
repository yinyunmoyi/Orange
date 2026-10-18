package com.example.orange.ui.reader

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LookupWebViewTest {

    @Test
    fun normalizeLookupSelection_acceptsPhraseWithSmartApostrophe() {
        assertEquals("they're after", normalizeLookupSelection("they\u2019re after"))
        assertEquals("don't", normalizeLookupSelection("don\u2018t"))
    }

    @Test
    fun normalizeLookupSelection_normalizesEpubSpacingAndHyphens() {
        assertEquals("look after", normalizeLookupSelection("\u00A0look\u202Fafter\u00A0"))
        assertEquals("state-of-the-art", normalizeLookupSelection("state\u2011of\u2010the-art"))
    }

    @Test
    fun normalizeLookupSelection_preservesExistingValidationLimits() {
        assertNull(normalizeLookupSelection("look after!"))
        assertNull(normalizeLookupSelection("one two three four five six seven eight nine"))
        assertNull(normalizeLookupSelection(" "))
    }
}
