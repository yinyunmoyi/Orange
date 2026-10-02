package com.example.orange.data.offline

import java.io.IOException
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OfflineLlmClientTest {
    @Test
    fun lookupTasks_generateIndependentlyWithoutResultCaching() = runBlocking {
        val prompts = mutableListOf<String>()
        val userPrompts = mutableListOf<String>()
        val responses = ArrayDeque(
            listOf(
                "\u8dd1\uff1b\u5954\u8dd1",
                "move quickly on foot",
                "\u8fd9\u91cc\u8868\u793a\u5feb\u901f\u8dd1\u52a8\u3002",
                "move quickly on foot",
                "\u8fd9\u91cc\u8868\u793a\u5feb\u901f\u8dd1\u52a8\u3002",
            ),
        )
        val client = OfflineLlmClient(OfflineTextGenerator { system, user, maxTokens ->
            prompts += system
            userPrompts += user
            assertEquals(if (user.startsWith("Word:")) 256 else 1_024, maxTokens)
            responses.removeFirst()
        })

        client.prepareLookup("run", false, "I run.", 2, 5).getOrThrow()

        assertEquals(
            "\u8dd1\uff1b\u5954\u8dd1",
            client.meaning().getOrThrow().meanings.single().meaning,
        )
        assertEquals("move quickly on foot", client.lookup().getOrThrow().definitions.single().text)
        assertEquals(
            "\u8fd9\u91cc\u8868\u793a\u5feb\u901f\u8dd1\u52a8\u3002",
            client.explain().getOrThrow().explanation,
        )
        client.lookup().getOrThrow()
        client.explain().getOrThrow()

        assertEquals(5, prompts.size)
        assertTrue(prompts[0].contains("纯文本"))
        assertTrue(prompts[1].contains("plain-English"))
        assertTrue(prompts[3].contains("plain-English"))
        assertEquals("单词：run\n/no_think", userPrompts[0])
        assertEquals("Word: run\n/no_think", userPrompts[1])
        assertEquals("目标单词：run\n语境：I 【run】.\n/no_think", userPrompts[2])
        assertTrue(prompts[2].contains("句中的角色"))
        assertTrue(prompts[2].contains("固定搭配或修饰结构"))
        assertTrue(prompts[2].contains("不要翻译整段"))
    }

    @Test
    fun lookupTaskFailure_doesNotFailOtherTasks() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            calls++
            when (calls) {
                1 -> "English only"
                2 -> "move quickly on foot"
                else -> "\u8fd9\u91cc\u8868\u793a\u5feb\u901f\u8dd1\u52a8\u3002"
            }
        })

        client.prepareLookup("run", false, "I run.", 2, 5).getOrThrow()

        assertTrue(client.meaning().isFailure)
        assertEquals("move quickly on foot", client.lookup().getOrThrow().definitions.single().text)
        assertEquals(
            "\u8fd9\u91cc\u8868\u793a\u5feb\u901f\u8dd1\u52a8\u3002",
            client.explain().getOrThrow().explanation,
        )
    }

    @Test
    fun failedLookupTask_isRetried() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            calls++
            if (calls == 1) {
                "English only"
            } else {
                "\u8dd1"
            }
        })

        client.prepareLookup("run", false, "I run.", 2, 5).getOrThrow()

        assertTrue(client.meaning().isFailure)
        assertEquals("\u8dd1", client.meaning().getOrThrow().meanings.single().meaning)
        assertEquals(2, calls)
    }

    @Test
    fun chineseMeaning_isRegeneratedForEveryRequest() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            calls++
            "\u8dd1"
        })

        client.prepareLookup("run", false, "I run.", 2, 5).getOrThrow()
        client.meaning().getOrThrow()
        client.prepareLookup("run", false, "They run.", 5, 8).getOrThrow()
        client.meaning().getOrThrow()

        assertEquals(2, calls)
    }

    @Test
    fun phraseMeaning_usesOneDedicatedPrompt() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { system, user, maxTokens ->
            calls++
            assertTrue(system.contains("纯文本"))
            assertTrue(system.contains("不能逐词拆解"))
            assertEquals("短语：look after\n/no_think", user)
            assertEquals(1_024, maxTokens)
            "\u7167\u987e\uff1b\u7167\u6599"
        })

        client.prepareLookup("look after", true, "Please look after her.", 7, 17).getOrThrow()

        assertEquals(
            "\u7167\u987e\uff1b\u7167\u6599",
            client.meaning().getOrThrow().meanings.single().meaning,
        )
        assertEquals(1, calls)
    }

    @Test
    fun chineseMeaning_acceptsPlainTextWithOptionalLabel() = runBlocking {
        val cases = listOf(
            Triple("magnetic", "\u78c1\u6027\u7684", "\u78c1\u6027\u7684"),
            Triple("sequins", "\u4e2d\u6587\u91ca\u4e49\uff1a\u4eae\u7247", "\u4eae\u7247"),
            Triple("pulled", "\u91ca\u4e49: \u62c9\u51fa\uff1b\u53d6\u51fa", "\u62c9\u51fa\uff1b\u53d6\u51fa"),
        )

        cases.forEach { (word, response, expected) ->
            val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ -> response })
            client.prepareLookup(word, false, word, 0, word.length).getOrThrow()

            assertEquals(expected, client.meaning().getOrThrow().meanings.single().meaning)
        }
    }

    @Test
    fun chineseMeaning_removesPartOfSpeechAndTargetEcho() = runBlocking {
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            "make head or tail of them \u5f04\u61c2\uff1b\u7406\u89e3\u6e05\u695a"
        })

        client.prepareLookup(
            "make head or tail of them",
            true,
            "I cannot make head or tail of them.",
            9,
            34,
        ).getOrThrow()
        val meaning = client.meaning().getOrThrow().meanings.single()

        assertEquals("", meaning.pos)
        assertEquals("\u5f04\u61c2\uff1b\u7406\u89e3\u6e05\u695a", meaning.meaning)
    }

    @Test
    fun chineseMeaning_rejectsCopiedSchemaPlaceholder() = runBlocking {
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            "\u4e2d\u6587\u91ca\u4e49"
        })

        client.prepareLookup("keypad", false, "Press the keypad.", 10, 16).getOrThrow()

        assertTrue(client.meaning().isFailure)
    }

    @Test
    fun englishLookup_usesPlainTextWithoutPartOfSpeech() = runBlocking {
        val client = OfflineLlmClient(OfflineTextGenerator { system, user, maxTokens ->
            assertTrue(system.contains("exact English headword"))
            assertTrue(system.contains("without using any reading context"))
            assertEquals("Word: sequins\n/no_think", user)
            assertEquals(256, maxTokens)
            "Definition: sequins small shiny discs sewn onto clothing"
        })

        client.prepareLookup(
            "sequins",
            false,
            "A purse with sequins.",
            13,
            20,
        ).getOrThrow()

        val lookup = client.lookup().getOrThrow()
        assertEquals("", lookup.definitions.single().pos)
        assertEquals("small shiny discs sewn onto clothing", lookup.definitions.single().text)
        assertEquals("", lookup.phonetic)
    }

    @Test
    fun sentenceTranslationAndAnalysis_generateSeparately() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, maxTokens ->
            calls++
            when {
                calls % 2 == 1 -> {
                    assertEquals(2_048, maxTokens)
                    "\u4f60\u597d\uff0c\u4e16\u754c\u3002"
                }
                else -> {
                    assertEquals(4_096, maxTokens)
                    """
                    {"s":"\u4e3b\u8c13\u7ed3\u6784","c":[
                      ["Hello","main","\u4e3b\u8bed","\u4f60\u597d"],
                      [" world.","other","\u8865\u5145","\u4e16\u754c"]
                    ]}
                    """.trimIndent()
                }
            }
        })

        val translation = client.translate("Hello world.").getOrThrow()
        val analysis = client.analyze("Hello world.").getOrThrow()
        client.translate("Hello world.").getOrThrow()
        client.analyze("Hello world.").getOrThrow()

        assertEquals("\u4f60\u597d\uff0c\u4e16\u754c\u3002", translation)
        assertEquals("Hello world.", analysis.chunks.joinToString("") { it.text })
        assertEquals(4, calls)
    }

    @Test
    fun malformedAnalysis_doesNotFailTranslation() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, maxTokens ->
            calls++
            if (maxTokens == 2_048) {
                "\u4ed6\u8bf4\uff1a\u201c\u4f60\u597d\u3002\u201d"
            } else {
                """{"s":"broken","c":[["He said "hello"." """
            }
        })

        assertEquals(
            "\u4ed6\u8bf4\uff1a\u201c\u4f60\u597d\u3002\u201d",
            client.translate("He said \"hello\".").getOrThrow(),
        )
        assertTrue(client.analyze("He said \"hello\".").isFailure)
        assertEquals(
            "\u4ed6\u8bf4\uff1a\u201c\u4f60\u597d\u3002\u201d",
            client.translate("He said \"hello\".").getOrThrow(),
        )
        assertEquals(4, calls)
    }

    @Test
    fun failedSentenceTask_isRetried() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            calls++
            if (calls == 1) throw IOException("temporary")
            "\u4f60\u597d\u3002"
        })

        assertTrue(client.translate("Hello.").isFailure)
        assertEquals("\u4f60\u597d\u3002", client.translate("Hello.").getOrThrow())
        assertEquals(2, calls)
    }

    @Test
    fun englishSentenceTranslation_isRejected() = runBlocking {
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ -> "Hello." })

        val result = client.translate("Hello.")

        assertTrue(result.isFailure)
    }

    @Test
    fun malformedAnalysis_isRetriedOnce() = runBlocking {
        var calls = 0
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            calls++
            if (calls == 1) {
                """{"s":"broken","c":[]}"""
            } else {
                """{"s":"\u4e3b\u8c13\u7ed3\u6784","c":[["Hello.","main","\u4e3b\u53e5","\u4f60\u597d\u3002"]]}"""
            }
        })

        assertEquals("Hello.", client.analyze("Hello.").getOrThrow().chunks.single().text)
        assertEquals(2, calls)
    }

    @Test
    fun sentenceAnalysis_repairsMissingCommaBetweenChunkFields() = runBlocking {
        var calls = 0
        val sentence =
            "As he was still a sophomore, he had quite a few courses left."
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            calls++
            """
            {"structure":"主句 + 修饰语","chunks":[
              {"text":"As he was still a sophomore","type":"adverbial","role":"状语","translation":"当他还是大二学生时"},
              {"text":", he had quite a few courses left." "type":"main","role":"主句","translation":"他还有不少课程"}
            ]}
            """.trimIndent()
        })

        val result = client.analyze(sentence).getOrThrow()

        assertEquals(sentence, result.chunks.joinToString("") { it.text })
        assertEquals(1, calls)
    }

    @Test
    fun sentenceAnalysis_retriesWhenChunkTranslationsCopyEnglish() = runBlocking {
        var calls = 0
        val sentence = "Tomohiko got goosebumps all over his body. We could really do this."
        val client = OfflineLlmClient(OfflineTextGenerator { system, _, _ ->
            calls++
            assertTrue(system.contains("translation 必须翻译对应 text"))
            if (calls == 1) {
                """
                {"structure":"主句+补充说明","chunks":[
                  {"text":"Tomohiko got goosebumps all over his body.","type":"main","role":"主句",
                   "translation":"Tomohiko got goosebumps all over his body."},
                  {"text":" We could really do this.","type":"main","role":"主句",
                   "translation":"We could really do this."}
                ]}
                """.trimIndent()
            } else {
                """
                {"structure":"主句+补充说明","chunks":[
                  {"text":"Tomohiko got goosebumps all over his body.","type":"main","role":"主句",
                   "translation":"友彦全身起了鸡皮疙瘩。"},
                  {"text":" We could really do this.","type":"main","role":"主句",
                   "translation":"我们真的能做到。"}
                ]}
                """.trimIndent()
            }
        })

        val result = client.analyze(sentence).getOrThrow()

        assertEquals(listOf("友彦全身起了鸡皮疙瘩。", "我们真的能做到。"), result.chunks.map { it.translation })
        assertEquals(2, calls)
    }

    @Test
    fun sentenceAnalysis_acceptsChineseObjectKeysAndDropsPlaceholderStructure() = runBlocking {
        val sentence = "Maybe not, but it definitely got an eyeful of the chatterbox."
        val client = OfflineLlmClient(OfflineTextGenerator { system, _, _ ->
            assertTrue(system.contains("structure"))
            assertTrue(system.contains("text、type、role、translation"))
            """
            {"s":"\u4e2d\u6587\u7ed3\u6784\u6982\u62ec","c":[
              {"\u539f\u6587\u7247\u6bb5":"Maybe not","\u7c7b\u578b":"\u72b6\u8bed",
               "\u4e2d\u6587\u8bed\u6cd5\u4f5c\u7528":"\u7701\u7565\u56de\u5e94","\u4e2d\u6587\u7247\u6bb5\u7ffb\u8bd1":"\u4e5f\u8bb8\u4e0d\u662f"},
              {"\u539f\u6587\u7247\u6bb5":", but it definitely got an eyeful of the chatterbox.",
               "\u7c7b\u578b":"\u4e3b\u5e72","\u4e2d\u6587\u8bed\u6cd5\u4f5c\u7528":"\u4e3b\u53e5",
               "\u4e2d\u6587\u7247\u6bb5\u7ffb\u8bd1":"\u4f46\u5b83\u786e\u5b9e\u770b\u5230\u4e86\u90a3\u4e2a\u8bdd\u5323\u5b50\u3002"}
            ]}
            """.trimIndent()
        })

        val result = client.analyze(sentence).getOrThrow()

        assertEquals("", result.structure)
        assertEquals(sentence, result.chunks.joinToString("") { it.text })
        assertEquals(listOf("adverbial", "main"), result.chunks.map { it.type })
    }

    @Test
    fun sentenceAnalysis_mapsUnknownChunkTypeAndAlignsSource() = runBlocking {
        val client = OfflineLlmClient(OfflineTextGenerator { _, _, _ ->
            """
            {"s":"\u4e3b\u8c13\u7ed3\u6784","c":[
              ["I","invented","\u4e3b\u8bed","\u6211"],
              ["wouldn\u2019t","modifier","\u8c13\u8bed","\u4e0d\u4f1a"],
              ["stop","main","\u52a8\u8bcd","\u505c\u4e0b"]
            ]}
            """.trimIndent()
        })

        val result = client.analyze("I wouldn't stop.").getOrThrow()

        assertEquals("I wouldn't stop.", result.chunks.joinToString("") { it.text })
        assertEquals("other", result.chunks.first().type)
    }
}
