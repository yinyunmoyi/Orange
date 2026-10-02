package com.example.orange.ui.reader

import com.example.orange.data.word.SentenceChunk
import org.junit.Assert.assertEquals
import org.junit.Test

class SentenceAnalyzeScreenTest {
    @Test
    fun visibleAnalysisChunks_hidesOnlySyntheticPunctuationAndItsLabel() {
        val chunks = listOf(
            SentenceChunk("They left", "main", "主句", "他们离开了"),
            SentenceChunk(",", "other", "补齐", ""),
            SentenceChunk(" but returned.", "main", "并列句", "但又回来了"),
            SentenceChunk(" Missing source text.", "other", "补齐", ""),
        )

        val visible = visibleAnalysisChunks(chunks)

        assertEquals(
            listOf("They left", " but returned.", " Missing source text."),
            visible.map { it.text },
        )
        assertEquals("", visible.last().role)
    }
}
