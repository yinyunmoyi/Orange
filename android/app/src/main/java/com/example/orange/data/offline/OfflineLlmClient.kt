package com.example.orange.data.offline

import com.example.orange.data.standalone.LlmResponseParser
import com.example.orange.data.standalone.StandaloneItemType
import com.example.orange.data.standalone.normalizeStandaloneText
import com.example.orange.data.word.ChineseMeaning
import com.example.orange.data.word.Definition
import com.example.orange.data.word.ExplainData
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import com.example.orange.data.word.SentenceAnalysis
import com.example.orange.data.word.SentenceChunk
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONArray
import org.json.JSONObject

fun interface OfflineTextGenerator {
    suspend fun generate(systemPrompt: String, userPrompt: String, maxTokens: Int): String
}

private data class OfflineLookupRequest(
    val text: String,
    val isPhrase: Boolean,
    val highlightedContext: String,
)

class OfflineLlmClient(
    private val generator: OfflineTextGenerator = OfflineTextGenerator(OfflineEngineSession::generate),
) {
    private val prepareMutex = Mutex()
    private val englishMutex = Mutex()
    private val meaningMutex = Mutex()
    private val explanationMutex = Mutex()
    private val translationMutex = Mutex()
    private val analysisMutex = Mutex()
    private var lookupRequest: OfflineLookupRequest? = null

    suspend fun prepareLookup(
        text: String,
        isPhrase: Boolean,
        context: String,
        start: Int,
        end: Int,
    ): Result<Unit> = prepareMutex.withLock {
        runCatching {
            val type = if (isPhrase) StandaloneItemType.PHRASE else StandaloneItemType.WORD
            val normalized = normalizeStandaloneText(type, text)
            val highlighted = highlight(context, normalized, start, end)
            lookupRequest = OfflineLookupRequest(
                text = normalized,
                isPhrase = isPhrase,
                highlightedContext = highlighted,
            )
            Unit
        }
    }

    suspend fun lookup(): Result<LookupData> = englishMutex.withLock {
        val request = requireLookupRequest()
        if (request.isPhrase) {
            return@withLock Result.success(LookupData(request.text, "", emptyList(), emptyList()))
        }
        val startedAt = debugElapsedMillis()
        val result = runCatching {
            val raw = generator.generate(
                systemPrompt = ENGLISH_DEFINITION_PROMPT,
                userPrompt = "Word: ${request.text}\n/no_think",
                maxTokens = MAX_ENGLISH_DEFINITION_TOKENS,
            )
            parseLookup(raw, request.text)
        }
        diagnoseTask("english_definition", startedAt, result)
        result
    }

    suspend fun meaning(): Result<MeaningData> = meaningMutex.withLock {
        val request = requireLookupRequest()
        val startedAt = debugElapsedMillis()
        val result = runCatching {
            val raw = generator.generate(
                systemPrompt = if (request.isPhrase) PHRASE_MEANING_PROMPT else WORD_MEANING_PROMPT,
                userPrompt = if (request.isPhrase) {
                    "短语：${request.text}\n/no_think"
                } else {
                    "单词：${request.text}\n/no_think"
                },
                maxTokens = MAX_LOOKUP_TASK_TOKENS,
            )
            parseMeaning(raw, request.text)
        }
        diagnoseTask("chinese_meaning", startedAt, result)
        result
    }

    suspend fun explain(): Result<ExplainData> = explanationMutex.withLock {
        val request = requireLookupRequest()
        val startedAt = debugElapsedMillis()
        val result = runCatching {
            val raw = generator.generate(
                systemPrompt = CONTEXT_EXPLANATION_PROMPT,
                userPrompt = "目标单词：${request.text}\n语境：${request.highlightedContext}\n/no_think",
                maxTokens = MAX_EXPLANATION_TOKENS,
            )
            val explanation = parsePlainText(raw, "本地模型语境解释无效")
            if (explanation.length > MAX_EXPLANATION_CHARS ||
                explanation.none(::isHanCharacter)
            ) {
                throw IOException("本地模型未返回中文语境解释")
            }
            ExplainData(request.text, explanation)
        }
        diagnoseTask("context_explanation", startedAt, result)
        result
    }

    suspend fun translate(sentence: String): Result<String> = translationMutex.withLock {
        normalizeStandaloneText(StandaloneItemType.SENTENCE, sentence)
        val source = sentence.trim()
        val startedAt = debugElapsedMillis()
        val result = runCatching {
            val raw = generator.generate(
                systemPrompt = SENTENCE_TRANSLATION_PROMPT,
                userPrompt = "$source\n/no_think",
                maxTokens = MAX_TRANSLATION_TOKENS,
            )
            val translation = parsePlainText(raw, "本地模型翻译无效")
            if (translation.length > MAX_TRANSLATION_CHARS ||
                translation.none(::isHanCharacter)
            ) {
                throw IOException("本地模型未返回中文翻译")
            }
            translation
        }
        diagnoseTask("sentence_translation", startedAt, result)
        result
    }

    suspend fun analyze(sentence: String): Result<SentenceAnalysis> = analysisMutex.withLock {
        normalizeStandaloneText(StandaloneItemType.SENTENCE, sentence)
        val source = sentence.trim()
        val startedAt = debugElapsedMillis()
        val result = runCatching {
            val raw = generator.generate(
                systemPrompt = SENTENCE_ANALYSIS_PROMPT,
                userPrompt = "句子：$source\n/no_think",
                maxTokens = MAX_ANALYSIS_TOKENS,
            )
            try {
                parseSentenceAnalysis(raw, source)
            } catch (error: Throwable) {
                if (error is CancellationException) throw error
                val retryRaw = generator.generate(
                    systemPrompt = SENTENCE_ANALYSIS_RETRY_PROMPT,
                    userPrompt = "句子：$source\n/no_think",
                    maxTokens = MAX_ANALYSIS_TOKENS,
                )
                try {
                    parseSentenceAnalysis(retryRaw, source)
                } catch (retryError: Throwable) {
                    retryError.addSuppressed(error)
                    throw retryError
                }
            }
        }
        diagnoseTask("sentence_analysis", startedAt, result)
        result
    }

    private fun requireLookupRequest(): OfflineLookupRequest =
        lookupRequest ?: throw IllegalStateException("offline lookup was not prepared")

    private fun parseLookup(raw: String, fallback: String): LookupData {
        val definition = sanitizePlainResult(
            parsePlainText(raw, "本地模型英文释义无效"),
            fallback,
            ENGLISH_DEFINITION_LABELS,
        )
        if (definition.isEmpty() ||
            definition.length > MAX_DEFINITION_CHARS ||
            definition.any(::isHanCharacter)
        ) {
            throw IOException("本地模型未返回有效英文释义")
        }
        return LookupData(
            word = fallback,
            phonetic = "",
            pronunciations = emptyList(),
            definitions = listOf(Definition("", definition)),
        )
    }

    private fun parseMeaning(raw: String, fallback: String): MeaningData {
        val meaning = sanitizePlainResult(
            parsePlainText(raw, "本地模型中文释义无效"),
            fallback,
            CHINESE_MEANING_LABELS,
        )
        if (meaning.isEmpty() ||
            meaning.length > MAX_DEFINITION_CHARS ||
            meaning.none(::isHanCharacter)
        ) {
            throw IOException("本地模型未返回有效中文释义")
        }
        return MeaningData(
            word = fallback,
            meanings = listOf(ChineseMeaning("", meaning)),
        )
    }

    private fun parseSentenceAnalysis(raw: String, sentence: String): SentenceAnalysis {
        val extracted = LlmResponseParser.extractJson(raw)
        val root = try {
            JSONObject(extracted)
        } catch (error: Throwable) {
            val repaired = repairCommonAnalysisJson(extracted)
            if (repaired == extracted) throw error
            try {
                JSONObject(repaired)
            } catch (repairError: Throwable) {
                repairError.addSuppressed(error)
                throw repairError
            }
        }
        val analysisRoot = root.optJSONObject("analysis")
            ?: root.optJSONObject("grammar")
            ?: root
        val structure = analysisRoot.optString(
            "structure",
            analysisRoot.optString(
                "s",
                analysisRoot.optString("中文结构概括", ""),
            ),
        ).trim().takeUnless(ANALYSIS_PLACEHOLDERS::contains).orEmpty()
        val chunks = analysisRoot.optJSONArray("chunks")
            ?: analysisRoot.optJSONArray("c")
            ?: analysisRoot.optJSONArray("segments")
            ?: analysisRoot.optJSONArray("components")
        val normalizedChunks = normalizeSentenceChunks(chunks)
        if (structure.length > MAX_FIELD_CHARS) {
            throw IOException("本地模型结构概括过长")
        }
        val normalized = JSONObject()
            .put("structure", structure)
            .put("chunks", normalizedChunks)
        val analysis = LlmResponseParser.parseSentenceAnalysis(
            normalized.toString(),
            sentence,
            requireExactAlignment = true,
        )
        if (analysis.chunks.any(::hasInvalidChunkTranslation)) {
            throw IOException("本地模型未返回中文片段翻译")
        }
        return analysis
    }

    private fun repairCommonAnalysisJson(raw: String): String =
        MISSING_CHUNK_FIELD_COMMA.replace(raw) { match ->
            "${match.groupValues[1]},${match.groupValues[2]}"
        }

    private fun parsePlainText(raw: String, invalidMessage: String): String {
        val value = LlmResponseParser.extractJson(raw).trim()
        val result = if (value.startsWith("{")) {
            val root = JSONObject(value)
            root.optString(
                "translation",
                root.optString("t", root.optString("explanation", root.optString("x", ""))),
            ).trim()
        } else {
            value
                .removePrefix("翻译：")
                .removePrefix("翻译:")
                .removePrefix("解释：")
                .removePrefix("解释:")
                .trim()
        }
        return result.ifEmpty { throw IOException(invalidMessage) }
    }

    private fun sanitizePlainResult(
        raw: String,
        target: String,
        labels: Set<String>,
    ): String {
        var value = raw.trim()
        for (label in labels) {
            if (value == label) return ""
            if (value.startsWith(label)) {
                val remainder = value.substring(label.length).trimMeaningSeparators()
                if (remainder.isNotEmpty()) value = remainder
            }
        }
        val candidate = value.trimStart('\"', '\'', '“', '”', '‘', '’', '【', '（', '(')
        if (candidate.startsWith(target, ignoreCase = true)) {
            val next = candidate.getOrNull(target.length)
            if (next == null || !next.isLetter() || isHanCharacter(next)) {
                val remainder = candidate.substring(target.length)
                    .trimStart('\"', '\'', '“', '”', '‘', '’', '】', '）', ')')
                    .trimMeaningSeparators()
                if (remainder.isNotEmpty()) value = remainder
            }
        }
        return value
    }

    private fun String.trimMeaningSeparators(): String =
        trim { character ->
            character.isWhitespace() ||
                character in charArrayOf(':', '：', ',', '，', ';', '；', '/', '-', '—')
        }

    private fun normalizeSentenceChunks(source: JSONArray?): JSONArray {
        val result = JSONArray()
        if (source == null) return result
        for (index in 0 until source.length()) {
            val value = source.opt(index)
            val chunk = when (value) {
                is JSONObject -> JSONObject()
                    .put(
                        "text",
                        value.firstRawString("text", "原文片段", "原文"),
                    )
                    .put(
                        "type",
                        normalizeChunkType(value.firstString("type", "类型")),
                    )
                    .put(
                        "role",
                        value.firstString("role", "中文语法作用", "语法作用"),
                    )
                    .put(
                        "translation",
                        value.firstString("translation", "中文片段翻译", "片段翻译", "翻译"),
                    )
                is JSONArray -> JSONObject()
                    .put("text", value.optString(0))
                    .put("type", normalizeChunkType(value.optString(1, "other")))
                    .put("role", value.optString(2))
                    .put("translation", value.optString(3))
                else -> throw IOException("本地模型 chunk 无效")
            }
            if (chunk.optString("role").length > MAX_FIELD_CHARS ||
                chunk.optString("translation").length > MAX_FIELD_CHARS
            ) {
                throw IOException("本地模型 chunk 字段无效")
            }
            result.put(chunk)
        }
        return result
    }

    private fun JSONObject.firstString(vararg keys: String): String =
        keys.firstNotNullOfOrNull { key ->
            optString(key, "").trim().takeIf(String::isNotEmpty)
        }.orEmpty()

    private fun JSONObject.firstRawString(vararg keys: String): String =
        keys.firstNotNullOfOrNull { key ->
            optString(key, "").takeIf(String::isNotEmpty)
        }.orEmpty()

    private fun normalizeChunkType(raw: String): String = when (raw.trim().lowercase()) {
        "主干", "主句" -> "main"
        "修饰", "修饰语" -> "modifier"
        "状语" -> "adverbial"
        "插入语" -> "parenthetical"
        "分词" -> "participle"
        "引语" -> "quote"
        "其它", "其他" -> "other"
        else -> raw.trim().lowercase()
    }

    private fun hasInvalidChunkTranslation(chunk: SentenceChunk): Boolean {
        if (chunk.role == "补齐") return false
        val wordCount = chunk.text.trim().split(Regex("\\s+")).count(String::isNotEmpty)
        if (wordCount <= 1) return false
        return chunk.translation.none(::isHanCharacter)
    }

    // #region debug-point A,D,E:independent-task-result
    private fun diagnoseTask(task: String, startedAt: Long, result: Result<*>) {
        val data = JSONObject()
            .put("runId", "post-fix")
            .put("hypothesisId", "A,D,E")
            .put("task", task)
            .put("durationMillis", debugElapsedMillis() - startedAt)
            .put("success", result.isSuccess)
        result.exceptionOrNull()?.let { error ->
            OfflineDiagnostics.failure("debug_offline_task_failed", null, error, data)
        } ?: OfflineDiagnostics.event("debug_offline_task_completed", null, data)
    }
    // #endregion

    private fun highlight(context: String, text: String, start: Int, end: Int): String {
        val normalizedContext = context.take(MAX_CONTEXT_CHARS)
        if (start >= 0 && end > start && end <= normalizedContext.length) {
            return normalizedContext.substring(0, start) + "【" +
                normalizedContext.substring(start, end) + "】" + normalizedContext.substring(end)
        }
        val index = normalizedContext.indexOf(text, ignoreCase = true)
        return if (index >= 0) {
            normalizedContext.substring(0, index) + "【" +
                normalizedContext.substring(index, index + text.length) + "】" +
                normalizedContext.substring(index + text.length)
        } else {
            normalizedContext
        }
    }

    private companion object {
        fun debugElapsedMillis(): Long = System.nanoTime() / 1_000_000L

        const val MAX_LOOKUP_TASK_TOKENS = 1_024
        const val MAX_ENGLISH_DEFINITION_TOKENS = 256
        const val MAX_EXPLANATION_TOKENS = 1_024
        const val MAX_TRANSLATION_TOKENS = 2_048
        const val MAX_ANALYSIS_TOKENS = 4_096
        const val MAX_CONTEXT_CHARS = 8_000
        const val MAX_EXPLANATION_CHARS = 16_000
        const val MAX_TRANSLATION_CHARS = 2_000
        const val MAX_DEFINITION_CHARS = 1_000
        const val MAX_FIELD_CHARS = 500
        val CHINESE_MEANING_LABELS = setOf("中文释义", "中文意思", "释义")
        val ENGLISH_DEFINITION_LABELS = setOf("English definition", "Definition")
        val ANALYSIS_PLACEHOLDERS = setOf("中文结构概括", "结构概括")
        val MISSING_CHUNK_FIELD_COMMA = Regex(
            """("(?:[^"\\]|\\.)*")\s+("(?:text|type|role|translation|原文片段|类型|中文语法作用|中文片段翻译)"\s*:)""",
        )
        const val ENGLISH_DEFINITION_PROMPT =
            """You are a professional monolingual English dictionary. Define only the exact English headword in the input, without using any reading context.
Return its most common modern meaning first. Separate additional common meanings with semicolons.
Output one concise plain-English line only. Do not output JSON, part of speech, phonetics, the headword, headings, examples, explanations, translations, or repeated text."""

        const val WORD_MEANING_PROMPT =
            """你是一部专业的英汉词典。给出输入英语单词本身的核心简体中文释义，不要结合任何阅读语境猜测含义。
如有多个常见义项，按现代英语使用频率由高到低排列并用中文分号分隔。
只输出一段简洁的中文纯文本，不输出 JSON、词性、英文原词、英文释义、标题、例句或解释。"""

        const val PHRASE_MEANING_PROMPT =
            """你是一部专业的英汉词典。给出输入英语短语本身作为整体的核心简体中文释义，不要结合任何阅读语境猜测含义，也不能逐词拆解。
如有多个常见义项，按现代英语使用频率由高到低排列并用中文分号分隔。
只输出一段简洁的中文纯文本，不输出 JSON、词性、英文短语、英文释义、标题、例句或解释。"""

        const val CONTEXT_EXPLANATION_PROMPT =
            """你是英语阅读理解助手。只围绕【】中的目标词结合上下文给出较详细的中文解释。
说明它在句中的角色、固定搭配或修饰结构，以及上下文暗示。只输出中文解释，不要翻译整段。"""

        const val SENTENCE_TRANSLATION_PROMPT =
            """你是英译中翻译器。只把输入英文翻译成自然、准确的简体中文。
只输出中文译文纯文本，不输出 JSON、引号、标题、分析或任何额外内容。"""

        const val SENTENCE_ANALYSIS_PROMPT =
            """你是英语句子结构分析器。只输出严格 JSON，不要整句翻译或 markdown。
输出 structure（30字内中文结构概括）和 chunks。chunks 按原句顺序粗粒度切分，每项含 text、type、role、translation。
structure、role 和 translation 必须使用简体中文；translation 必须翻译对应 text，禁止复制英文原文。
所有 text 拼接必须完全等于原句。type 仅可为 main、modifier、adverbial、parenthetical、participle、quote、other。"""

        const val SENTENCE_ANALYSIS_RETRY_PROMPT =
            """你是英语句子结构分析器。上一次输出格式无效，请重新分析。
只输出一个可被标准 JSON 解析器直接解析的对象，包含 structure 和 chunks。
chunks 必须是一个数组，每项必须是含 text、type、role、translation 的对象；对象之间必须有逗号，字符串中的引号必须转义，禁止额外数组或 markdown。
structure、role 和 translation 必须使用简体中文；translation 必须翻译对应 text，禁止复制英文原文。
所有 text 按顺序拼接必须完全等于原句。type 仅可为 main、modifier、adverbial、parenthetical、participle、quote、other。"""
    }
}

private fun isHanCharacter(character: Char): Boolean =
    Character.UnicodeScript.of(character.code) == Character.UnicodeScript.HAN
