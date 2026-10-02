package com.example.orange.data.standalone

import com.example.orange.data.word.ChineseMeaning
import com.example.orange.data.word.Definition
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import com.example.orange.data.word.SentenceAnalysis
import com.example.orange.data.word.SentenceChunk
import java.io.IOException
import java.util.Locale
import org.json.JSONObject

object LlmResponseParser {
    fun parseLookup(content: String, fallback: String): LookupData {
        val root = JSONObject(extractJson(content))
        val definitions = mutableListOf<Definition>()
        root.optJSONArray("definitions")?.let { array ->
            for (index in 0 until array.length()) {
                val item = array.optJSONObject(index) ?: continue
                val text = item.optString("definition", "").trim()
                if (text.isNotEmpty()) {
                    definitions += Definition(
                        item.optString("partOfSpeech", "").trim(),
                        text,
                    )
                }
            }
        }
        if (definitions.isEmpty()) throw IOException("llm dictionary returned no definitions")
        return LookupData(
            word = root.optString("word", fallback).trim().ifEmpty { fallback },
            phonetic = root.optString("phonetic", "").trim(),
            pronunciations = emptyList(),
            definitions = definitions,
        )
    }

    fun parseMeaning(content: String, fallback: String): MeaningData {
        val root = JSONObject(extractJson(content))
        val meanings = mutableListOf<ChineseMeaning>()
        root.optJSONArray("meanings")?.let { array ->
            for (index in 0 until array.length()) {
                val item = array.optJSONObject(index) ?: continue
                val text = item.optString("meaning", "").trim()
                if (text.isNotEmpty()) {
                    meanings += ChineseMeaning(
                        item.optString("partOfSpeech", "").trim(),
                        text,
                    )
                }
            }
        }
        if (meanings.isEmpty()) throw IOException("llm returned no meanings")
        return MeaningData(root.optString("word", fallback).ifBlank { fallback }, meanings)
    }

    fun parseSentenceAnalysis(
        content: String,
        sentence: String,
        requireExactAlignment: Boolean = false,
    ): SentenceAnalysis {
        val root = JSONObject(extractJson(content))
        val chunks = mutableListOf<SentenceChunk>()
        root.optJSONArray("chunks")?.let { array ->
            for (index in 0 until array.length()) {
                val item = array.optJSONObject(index) ?: continue
                val text = item.optString("text", "")
                if (text.isEmpty()) continue
                val rawType = item.optString("type", "other").trim().lowercase(Locale.ROOT)
                chunks += SentenceChunk(
                    text = text,
                    type = rawType.takeIf(ALLOWED_CHUNK_TYPES::contains) ?: "other",
                    role = item.optString("role", "").trim(),
                    translation = item.optString("translation", "").trim(),
                )
            }
        }
        if (chunks.isEmpty() || chunks.size > MAX_CHUNKS) {
            throw IOException("llm sentence chunks are invalid")
        }
        val aligned = alignChunksToSentence(sentence, chunks)
        if (requireExactAlignment && aligned.joinToString("") { it.text } != sentence) {
            throw IOException("llm sentence chunks do not match source")
        }
        return SentenceAnalysis(
            sentence = sentence,
            structure = root.optString("structure", "").trim(),
            chunks = aligned,
        )
    }

    fun extractJson(raw: String): String {
        var value = raw.trim()
        value = THINK_BLOCK.replace(value, "").trim()
        if (value.startsWith("```")) {
            value = value.removePrefix("```").removePrefix("json").removePrefix("JSON")
            value = value.substringBeforeLast("```").trim()
        }
        val start = value.indexOf('{')
        val end = value.lastIndexOf('}')
        return if (start >= 0 && end > start) value.substring(start, end + 1) else value
    }

    private fun alignChunksToSentence(
        sentence: String,
        chunks: List<SentenceChunk>,
    ): List<SentenceChunk> {
        val aligned = mutableListOf<SentenceChunk>()
        var cursor = 0
        for (chunk in chunks) {
            if (cursor >= sentence.length) break
            val match = locateChunk(sentence, cursor, chunk.text) ?: continue
            var start = match.first
            val end = match.second
            if (start > cursor) {
                val gap = sentence.substring(cursor, start)
                if (gap.all(Char::isWhitespace)) {
                    start = cursor
                } else {
                    aligned += SentenceChunk(
                        text = gap,
                        type = "other",
                        role = "补齐",
                        translation = "",
                    )
                }
            }
            aligned += chunk.copy(text = sentence.substring(start, end))
            cursor = end
        }
        if (cursor < sentence.length) {
            val remainder = sentence.substring(cursor)
            if (remainder.all(Char::isWhitespace) && aligned.isNotEmpty()) {
                val last = aligned.lastIndex
                aligned[last] = aligned[last].copy(text = aligned[last].text + remainder)
            } else {
                aligned += SentenceChunk(
                    text = remainder,
                    type = "other",
                    role = "补齐",
                    translation = "",
                )
            }
        }
        return aligned
    }

    private fun locateChunk(source: String, start: Int, candidate: String): Pair<Int, Int>? {
        val needle = candidate.filterNot(Char::isWhitespace)
        if (needle.isEmpty()) return null
        val sourceIndexes = mutableListOf<Int>()
        val sourceCharacters = mutableListOf<Char>()
        for (index in start until source.length) {
            if (!source[index].isWhitespace()) {
                sourceIndexes += index
                sourceCharacters += source[index]
            }
        }
        if (needle.length > sourceCharacters.size) return null
        for (viewStart in 0..sourceCharacters.size - needle.length) {
            var matches = true
            for (offset in needle.indices) {
                if (!equivalentCharacter(needle[offset], sourceCharacters[viewStart + offset])) {
                    matches = false
                    break
                }
            }
            if (matches) {
                return sourceIndexes[viewStart] to
                    (sourceIndexes[viewStart + needle.lastIndex] + 1)
            }
        }
        return null
    }

    private fun equivalentCharacter(left: Char, right: Char): Boolean =
        when {
            left == right -> true
            left.lowercaseChar() == right.lowercaseChar() -> true
            left in APOSTROPHES && right in APOSTROPHES -> true
            left in DOUBLE_QUOTES && right in DOUBLE_QUOTES -> true
            left in DASHES && right in DASHES -> true
            else -> false
        }

    private val THINK_BLOCK = Regex("<think>.*?</think>", setOf(RegexOption.DOT_MATCHES_ALL, RegexOption.IGNORE_CASE))
    private val ALLOWED_CHUNK_TYPES = setOf(
        "main", "modifier", "adverbial", "parenthetical", "participle", "quote", "other",
    )
    private val APOSTROPHES = setOf('\'', '\u2018', '\u2019')
    private val DOUBLE_QUOTES = setOf('"', '\u201C', '\u201D')
    private val DASHES = setOf('-', '\u2013', '\u2014')
    private const val MAX_CHUNKS = 64
}
