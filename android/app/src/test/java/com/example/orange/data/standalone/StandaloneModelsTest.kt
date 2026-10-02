package com.example.orange.data.standalone

import com.example.orange.data.word.FavoriteItem
import com.example.orange.data.word.SentenceFavorite
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

class StandaloneModelsTest {
    @Test
    fun normalizeStandaloneText_matchesItemSemantics() {
        assertEquals("hello", normalizeStandaloneText(StandaloneItemType.WORD, "  HeLLo "))
        assertEquals("look up", normalizeStandaloneText(StandaloneItemType.PHRASE, " Look   Up "))
        assertEquals(
            "The quick fox.",
            normalizeStandaloneText(StandaloneItemType.SENTENCE, " The  quick\nfox. "),
        )
    }

    @Test(expected = IllegalArgumentException::class)
    fun normalizeStandaloneText_rejectsBlankText() {
        normalizeStandaloneText(StandaloneItemType.WORD, " \n ")
    }

    @Test
    fun sentenceKey_isStableAfterWhitespaceNormalization() {
        assertEquals(
            standaloneSentenceKey("One  sentence.\n"),
            standaloneSentenceKey(" One sentence. "),
        )
        assertNotEquals(
            standaloneSentenceKey("One sentence."),
            standaloneSentenceKey("one sentence."),
        )
    }

    @Test
    fun mergeStandaloneWords_keepsLocalMarkerUntilSyncCleanup() {
        val remote = listOf(FavoriteItem("word", 7, "hello", "", "", "remote"))
        val local = listOf(
            FavoriteItem("word", 0, "HELLO", "", "", "local", clientId = "client-1"),
            FavoriteItem("phrase", 0, "look up", "", "", "search", clientId = "client-2"),
        )

        val merged = mergeStandaloneWords(remote, local)

        assertEquals(listOf("HELLO", "look up"), merged.map { it.text })
        assertEquals(listOf("client-1", "client-2"), merged.map { it.clientId })
    }

    @Test
    fun mergeStandaloneSentences_keepsLocalMarkerUntilSyncCleanup() {
        val remote = listOf(SentenceFavorite(9, "A sentence.", "remote", ""))
        val local = listOf(
            SentenceFavorite(0, " A  sentence. ", "local", "", clientId = "client-1"),
            SentenceFavorite(0, "Another.", "another", "", clientId = "client-2"),
        )

        val merged = mergeStandaloneSentences(remote, local)

        assertEquals(listOf(" A  sentence. ", "Another."), merged.map { it.sentence })
    }

    @Test
    fun mergeStandaloneSentences_acceptsHistoricalTypography() {
        val sentence = "‘Hello …’ she said – quietly."
        val remote = listOf(SentenceFavorite(9, sentence, "remote", ""))

        val merged = mergeStandaloneSentences(remote, emptyList())

        assertEquals(listOf(sentence), merged.map { it.sentence })
    }

    @Test
    fun snapshot_countsMappedFavoriteUntilChildrenFinishSyncing() {
        val favorite = StandaloneFavorite(
            clientId = "client-1",
            itemType = StandaloneItemType.WORD,
            text = "hello",
            displayJson = "{}",
            translation = "",
            createdAt = 1L,
            syncState = StandaloneSyncState.MAPPED,
            serverId = 7L,
            lastError = null,
            retryCount = 0,
        )

        assertEquals(1, StandaloneSnapshot(listOf(favorite)).pendingCount)
    }
}
