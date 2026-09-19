package com.example.orange.ui.wordlist

import com.example.orange.data.word.ConfusableWord
import com.example.orange.data.word.WordAssociationItem
import com.example.orange.data.word.sanitizeConfusableWords
import com.example.orange.data.word.sanitizeWordAssociations
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ConfusableWordsScreenTest {

    @Test
    fun normalizeConfusableQuery_trimsAndLowercasesValidWords() {
        assertEquals("affect", normalizeConfusableQuery("  AfFeCt  "))
        assertEquals("well-being", normalizeConfusableQuery("well-being"))
        assertEquals("can't", normalizeConfusableQuery("can't"))
    }

    @Test
    fun normalizeConfusableQuery_rejectsInvalidInput() {
        assertNull(normalizeConfusableQuery(""))
        assertNull(normalizeConfusableQuery("look up"))
        assertNull(normalizeConfusableQuery("word2"))
        assertNull(normalizeConfusableQuery("word!"))
    }

    @Test
    fun sanitizeConfusableWords_removesInvalidRowsAndKeepsBlankMeaning() {
        val valid = ConfusableWord(itemId = 7, word = "affect", topMeaningText = "")
        val result = sanitizeConfusableWords(
            listOf(
                valid,
                ConfusableWord(itemId = 0, word = "affection", topMeaningText = "喜爱"),
                ConfusableWord(itemId = 8, word = " ", topMeaningText = "空"),
            ),
        )

        assertEquals(listOf(valid), result)
    }

    @Test
    fun associationHintLimit_countsUnicodeCodePoints() {
        assertTrue(acceptsAssociationHint("意".repeat(200)))
        assertFalse(acceptsAssociationHint("意".repeat(201)))
        assertEquals(1, associationHintLength("\uD83D\uDE00"))
    }

    @Test
    fun sanitizeWordAssociations_filtersBlankWordsAndKeepsServerOrder() {
        val first = WordAssociationItem("effect", "效果", 0.9)
        val second = WordAssociationItem("affection", "喜爱", 0.8)

        val result = sanitizeWordAssociations(
            listOf(
                first,
                WordAssociationItem(" ", "无效", 1.0),
                second,
            ),
        )

        assertEquals(listOf(first, second), result)
    }

    @Test
    fun associationAlreadyInGroup_matchesCaseInsensitivelyAfterTrimming() {
        val group = listOf(
            ConfusableWord(itemId = 7, word = " Affect ", topMeaningText = "影响"),
            ConfusableWord(itemId = 8, word = "effect", topMeaningText = "效果"),
        )

        assertTrue(
            associationAlreadyInGroup(
                WordAssociationItem("AFFECT", "影响", 0.9),
                group,
            ),
        )
        assertFalse(
            associationAlreadyInGroup(
                WordAssociationItem("affection", "喜爱", 0.8),
                group,
            ),
        )
    }

    @Test
    fun associationAlreadyInGroup_returnsFalseForEmptyGroupThenTrueAfterRefresh() {
        val candidate = WordAssociationItem("affection", "喜爱", 0.8)
        assertFalse(associationAlreadyInGroup(candidate, emptyList()))

        val refreshed = listOf(
            ConfusableWord(itemId = 9, word = "affection", topMeaningText = "喜爱"),
        )
        assertTrue(associationAlreadyInGroup(candidate, refreshed))
    }
}
