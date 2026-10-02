package com.example.orange.data.standalone

import com.example.orange.BuildConfig
import com.example.orange.data.logging.AppLog as Log
import com.example.orange.data.network.NetworkAccessPolicy
import com.example.orange.data.word.ChineseMeaning
import com.example.orange.data.word.Definition
import com.example.orange.data.word.ExplainData
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import com.example.orange.data.word.SentenceAnalysis
import com.example.orange.data.word.SentenceChunk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.util.Locale

object StandaloneLlmClient {
    suspend fun lookup(word: String): Result<LookupData> = requestResult {
        val normalized = normalizeStandaloneText(StandaloneItemType.WORD, word)
        val content = complete(
            system = WORD_PROMPT,
            user = "单词：$normalized",
            maxTokens = 4096,
            responseFormat = "json_object",
        )
        parseLookup(content, normalized)
    }

    suspend fun meaning(word: String): Result<MeaningData> = requestResult {
        val normalized = normalizeStandaloneText(StandaloneItemType.WORD, word)
        parseMeaning(
            complete(MEANING_PROMPT, "单词：$normalized", 4096, "json_object"),
            normalized,
        )
    }

    suspend fun phrase(phrase: String): Result<MeaningData> = requestResult {
        val normalized = normalizeStandaloneText(StandaloneItemType.PHRASE, phrase)
        parseMeaning(
            complete(PHRASE_PROMPT, "短语：$normalized", 2048, "json_object"),
            normalized,
        )
    }

    suspend fun explain(
        word: String,
        context: String,
        wordStart: Int,
        wordEnd: Int,
    ): Result<ExplainData> = requestResult {
        val highlighted = highlight(context, word, wordStart, wordEnd)
        val explanation = complete(
            EXPLAIN_PROMPT,
            "目标单词：$word\n语境：$highlighted",
            4096,
            "text",
        ).trim()
        if (explanation.isEmpty()) throw IOException("empty explanation")
        ExplainData(word.lowercase(Locale.ROOT), explanation)
    }

    suspend fun analyzeSentence(sentence: String): Result<SentenceAnalysis> = requestResult {
        parseSentenceAnalysis(
            complete(SENTENCE_PROMPT, "句子：$sentence", 8192, "json_object"),
            sentence,
        )
    }

    suspend fun translateSentenceStream(
        sentence: String,
        onDelta: (String) -> Unit,
        onDone: () -> Unit,
        onError: (String) -> Unit,
    ) = withContext(Dispatchers.IO) {
        var emitted = false
        var lastError: Throwable? = null
        for (attempt in 0..1) {
            try {
                stream(TRANSLATE_PROMPT, "句子：$sentence", 4096) {
                    emitted = true
                    onDelta(it)
                }
                onDone()
                return@withContext
            } catch (error: Throwable) {
                lastError = error
                if (emitted || attempt == 1) {
                    onError(error.message ?: error.javaClass.simpleName)
                    return@withContext
                }
                Thread.sleep(350L)
            }
        }
        onError(lastError?.message ?: "llm translation failed")
    }

    private suspend fun <T> requestResult(block: () -> T): Result<T> = withContext(Dispatchers.IO) {
        runCatching(block)
    }

    private fun complete(
        system: String,
        user: String,
        maxTokens: Int,
        responseFormat: String,
    ): String {
        val request = requestBody(system, user, maxTokens, stream = false)
            .put("response_format", JSONObject().put("type", responseFormat))
        val response = executeWithRetry(request)
        val content = response.optJSONArray("choices")
            ?.optJSONObject(0)
            ?.optJSONObject("message")
            ?.optString("content", "")
            .orEmpty()
        if (content.isBlank()) throw IOException("llm returned empty content")
        return extractJson(content)
    }

    private fun stream(
        system: String,
        user: String,
        maxTokens: Int,
        onDelta: (String) -> Unit,
    ) {
        ensureConfigured()
        val connection = openConnection()
        var received = false
        try {
            connection.setRequestProperty("Accept", "text/event-stream")
            val payload = requestBody(system, user, maxTokens, stream = true).toString()
            connection.outputStream.use { it.write(payload.toByteArray(Charsets.UTF_8)) }
            val code = connection.responseCode
            if (code !in 200..299) throw httpError(connection, code)
            BufferedReader(InputStreamReader(connection.inputStream, Charsets.UTF_8)).use { reader ->
                while (true) {
                    val line = reader.readLine() ?: break
                    if (!line.startsWith("data:")) continue
                    val data = line.substringAfter("data:").trim()
                    if (data == "[DONE]") break
                    val delta = runCatching {
                        JSONObject(data).optJSONArray("choices")
                            ?.optJSONObject(0)
                            ?.optJSONObject("delta")
                            ?.optString("content", "")
                            .orEmpty()
                    }.getOrDefault("")
                    if (delta.isNotEmpty()) {
                        received = true
                        onDelta(delta)
                    }
                }
            }
            if (!received) throw IOException("llm returned empty translation")
        } finally {
            NetworkAccessPolicy.release(connection)
        }
    }

    private fun executeWithRetry(body: JSONObject): JSONObject {
        var lastError: Throwable? = null
        repeat(2) { attempt ->
            try {
                return execute(body)
            } catch (error: Throwable) {
                lastError = error
                if (attempt == 1 || error is IllegalArgumentException) throw error
                Thread.sleep(350L)
            }
        }
        throw lastError ?: IOException("llm request failed")
    }

    private fun execute(body: JSONObject): JSONObject {
        ensureConfigured()
        val connection = openConnection()
        try {
            connection.setRequestProperty("Accept", "application/json")
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            val code = connection.responseCode
            val text = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                Log.w(TAG, "request failed code=$code")
                throw IOException("llm HTTP $code")
            }
            return JSONObject(text)
        } finally {
            NetworkAccessPolicy.release(connection)
        }
    }

    private fun openConnection(): HttpURLConnection =
        NetworkAccessPolicy.openConnection(URL(BuildConfig.ANDROID_LLM_API_URL)).apply {
            requestMethod = "POST"
            doOutput = true
            doInput = true
            connectTimeout = 8_000
            readTimeout = 180_000
            setRequestProperty("Content-Type", "application/json; charset=utf-8")
            setRequestProperty("Authorization", "Bearer ${BuildConfig.ANDROID_LLM_API_KEY}")
            setRequestProperty("Connection", "close")
        }

    private fun requestBody(
        system: String,
        user: String,
        maxTokens: Int,
        stream: Boolean,
    ): JSONObject = JSONObject()
        .put("model", BuildConfig.ANDROID_LLM_MODEL)
        .put(
            "messages",
            JSONArray()
                .put(JSONObject().put("role", "system").put("content", system))
                .put(JSONObject().put("role", "user").put("content", user)),
        )
        .put("temperature", 0.3)
        .put("max_tokens", maxTokens)
        .put("stream", stream)
        .put("thinking", JSONObject().put("type", "disabled"))

    private fun ensureConfigured() {
        if (BuildConfig.ANDROID_LLM_API_KEY.isBlank()) {
            throw IllegalStateException("ANDROID_LLM_API_KEY is not configured")
        }
        if (!BuildConfig.ANDROID_LLM_API_URL.startsWith("https://")) {
            throw IllegalStateException("ANDROID_LLM_API_URL must use HTTPS")
        }
    }

    private fun httpError(connection: HttpURLConnection, code: Int): IOException {
        connection.errorStream?.close()
        Log.w(TAG, "stream failed code=$code")
        return IOException("llm HTTP $code")
    }

    internal fun parseLookup(content: String, fallback: String): LookupData =
        LlmResponseParser.parseLookup(content, fallback)

    internal fun parseMeaning(content: String, fallback: String): MeaningData =
        LlmResponseParser.parseMeaning(content, fallback)

    internal fun parseSentenceAnalysis(content: String, sentence: String): SentenceAnalysis =
        LlmResponseParser.parseSentenceAnalysis(content, sentence)

    internal fun extractJson(raw: String): String = LlmResponseParser.extractJson(raw)

    private fun highlight(context: String, word: String, start: Int, end: Int): String {
        if (start >= 0 && end > start && end <= context.length &&
            context.substring(start, end).equals(word, ignoreCase = true)
        ) {
            return context.substring(0, start) + "【" + context.substring(start, end) + "】" +
                context.substring(end)
        }
        val index = context.indexOf(word, ignoreCase = true)
        return if (index >= 0) {
            context.substring(0, index) + "【" + context.substring(index, index + word.length) +
                "】" + context.substring(index + word.length)
        } else {
            context
        }
    }

    private const val TAG = "StandaloneLlm"
    private const val WORD_PROMPT =
        """You are a concise English dictionary. Return strict JSON only.
Output {"word":"...","phonetic":"...","definitions":[{"partOfSpeech":"noun","definition":"..."}]}.
Definitions must be English, ordered by modern usage frequency. Do not include examples or audio URLs."""

    private const val MEANING_PROMPT =
        """你是一部专业的英汉词典。给出英语单词的核心汉语释义，按现代英语使用频率排序，标注简写词性。
只输出严格 JSON：{"meanings":[{"partOfSpeech":"n.","meaning":"信条、教义"}]}。不要例句或解释。"""

    private const val PHRASE_PROMPT =
        """你是一部专业的英汉词典。给出英语短语作为整体的核心汉语释义，按使用频率排序。
只输出严格 JSON：{"meanings":[{"partOfSpeech":"短语动词","meaning":"期待、盼望"}]}。不要逐词翻译、例句或解释。"""

    private const val EXPLAIN_PROMPT =
        """你是英语阅读理解助手。只围绕【】中的目标词结合上下文给出较详细的中文解释。
说明它在句中的角色、固定搭配或修饰结构，以及上下文暗示。只输出中文解释，不要翻译整段。"""

    private const val SENTENCE_PROMPT =
        """你是英语句子结构分析器。只输出严格 JSON，不要整句翻译或 markdown。
输出 structure（30字内中文结构概括）和 chunks。chunks 按原句顺序粗粒度切分，每项含 text、type、role、translation。
所有 text 拼接必须完全等于原句。type 仅可为 main、modifier、adverbial、parenthetical、participle、quote、other。"""

    private const val TRANSLATE_PROMPT =
        """你是一名英语翻译。直接返回完整、自然、通顺的中文译文，不要解释、JSON、markdown、引号或标签。"""
}
