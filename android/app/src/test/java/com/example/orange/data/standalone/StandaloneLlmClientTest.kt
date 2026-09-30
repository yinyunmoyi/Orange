package com.example.orange.data.standalone

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class StandaloneLlmClientTest {
    @Test
    fun parseLookup_readsStrictDictionaryShapeAndNeverCreatesAudio() {
        val result = StandaloneLlmClient.parseLookup(
            """
            {"word":"run","phonetic":"/rʌn/","definitions":[
              {"partOfSpeech":"verb","definition":"move quickly"},
              {"partOfSpeech":"noun","definition":"an act of running"}
            ]}
            """.trimIndent(),
            "run",
        )

        assertEquals("run", result.word)
        assertEquals("/rʌn/", result.phonetic)
        assertEquals(listOf("move quickly", "an act of running"), result.definitions.map { it.text })
        assertTrue(result.pronunciations.isEmpty())
    }

    @Test
    fun extractJson_removesMarkdownFence() {
        assertEquals(
            """{"meanings":[]}""",
            StandaloneLlmClient.extractJson("```json\n{\"meanings\":[]}\n```"),
        )
    }

    @Test
    fun parseSentenceAnalysis_normalizesUnknownChunkType() {
        val result = StandaloneLlmClient.parseSentenceAnalysis(
            """
            {"structure":"main","chunks":[
              {"text":"Hello","type":"unexpected","role":"main","translation":"hello"},
              {"text":" world.","type":"main","role":"complement","translation":"world"}
            ]}
            """.trimIndent(),
            "Hello world.",
        )

        assertEquals(listOf("other", "main"), result.chunks.map { it.type })
        assertEquals("Hello world.", result.chunks.joinToString("") { it.text })
    }

    @Test
    fun parseMeaning_ignoresEmptyEntries() {
        val result = StandaloneLlmClient.parseMeaning(
            """
            {"meanings":[
              {"partOfSpeech":"v.","meaning":"run"},
              {"partOfSpeech":"n.","meaning":" "}
            ]}
            """.trimIndent(),
            "run",
        )

        assertEquals(1, result.meanings.size)
        assertEquals("run", result.meanings.single().meaning)
    }

    @Test
    fun parseSentenceAnalysis_alignsWhitespaceToSource() {
        val result = StandaloneLlmClient.parseSentenceAnalysis(
            """
            {"structure":"main","chunks":[
              {"text":"Hello ","type":"main","role":"main","translation":"hello"},
              {"text":"world.","type":"modifier","role":"object","translation":"world"}
            ]}
            """.trimIndent(),
            "Hello  world.",
        )

        assertEquals("Hello  world.", result.chunks.joinToString("") { it.text })
    }

    @Test
    fun parseSentenceAnalysis_keepsGrammarWhenChunksDoNotMatchSource() {
        val result = StandaloneLlmClient.parseSentenceAnalysis(
            """{"structure":"主谓结构","chunks":[{"text":"Changed","type":"main","role":"主句","translation":"改变"}]}""",
            "Original",
        )

        assertEquals("Original", result.sentence)
        assertEquals("主谓结构", result.structure)
        assertEquals("Changed", result.chunks.single().text)
        assertEquals("主句", result.chunks.single().role)
    }
}
