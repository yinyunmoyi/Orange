package com.example.orange.data.word

import com.example.orange.data.logging.AppLog as Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.util.Locale

data class Pronunciation(
    val accent: String,
    val text: String,
    val audioUrl: String,
)

data class Definition(
    val pos: String,
    val text: String,
)

data class LookupData(
    val word: String,
    val phonetic: String,
    val pronunciations: List<Pronunciation>,
    val definitions: List<Definition>,
    val pos: String = "",
    val collins: Int = 0,
    val oxford: Int = 0,
    val tags: List<String> = emptyList(),
    val exchange: String = "",
)

data class ChineseMeaning(
    val pos: String,
    val meaning: String,
)

data class MeaningData(
    val word: String,
    val meanings: List<ChineseMeaning>,
    val pos: String = "",
    val collins: Int = 0,
    val oxford: Int = 0,
    val tags: List<String> = emptyList(),
)

data class ConfusableWord(
    val itemId: Long,
    val word: String,
    val topMeaningText: String,
)

data class ConfusableWordsData(
    val groupId: Long,
    val members: List<ConfusableWord>,
)

internal fun sanitizeConfusableWords(items: List<ConfusableWord>): List<ConfusableWord> =
    items.filter { it.itemId > 0L && it.word.isNotBlank() }

data class WordAssociationItem(
    val word: String,
    val topMeaningText: String,
    val similarity: Double,
)

data class WordAssociationData(
    val items: List<WordAssociationItem>,
)

internal fun sanitizeWordAssociations(items: List<WordAssociationItem>): List<WordAssociationItem> =
    items.filter { it.word.isNotBlank() }

data class ExplainData(
    val word: String,
    val explanation: String,
)

data class SentenceChunk(
    val text: String,
    val type: String,
    val role: String,
    val translation: String,
)

data class SentenceAnalysis(
    val sentence: String,
    val structure: String,
    val chunks: List<SentenceChunk>,
)

data class SentenceFavorite(
    val id: Long,
    val sentence: String,
    val translation: String,
    val createdAt: String,
    val note: String = "",
    val tags: List<SentenceFavoriteTag> = emptyList(),
    val clientId: String? = null,
    val syncState: String? = null,
)

data class SentenceTag(
    val id: Long,
    val name: String,
    val color: String,
    val createdAt: String,
)

data class SentenceFavoriteTag(
    val id: Long,
    val name: String,
    val color: String,
)

data class SentenceFavoriteStatus(
    val favorited: Boolean,
    val id: Long,
)

data class SentenceFavoriteListData(
    val items: List<SentenceFavorite>,
    val total: Int,
)

internal fun sanitizeSentenceFavorites(items: List<SentenceFavorite>): List<SentenceFavorite> =
    items.filter { it.id > 0L && it.sentence.isNotBlank() }

internal fun buildSentenceFavoritesUrl(tagIds: Set<Long>): String {
    val base = WordServiceConfig.origin + "/api/v1/sentences/favorites"
    val ids = tagIds.filter { it > 0L }.sorted()
    return if (ids.isEmpty()) base else "$base?tagIds=${ids.joinToString(",")}"
}

data class FavoriteItem(
    val itemType: String,
    val itemId: Long,
    val text: String,
    val createdAt: String,
    val topMeaningPos: String,
    val topMeaningText: String,
    val level: Int = 0,
    val clientId: String? = null,
    val syncState: String? = null,
)

data class FavoriteListData(
    val items: List<FavoriteItem>,
    val total: Int,
)

data class FavoriteGroupSummary(
    val id: String,
    val type: String,
    val rangeStart: String,
    val rangeEnd: String,
    val displayStart: String,
    val displayEnd: String,
    val count: Int,
)

data class FavoriteGroupListData(
    val groups: List<FavoriteGroupSummary>,
    val total: Int,
)

data class FavoritePageData(
    val items: List<FavoriteItem>,
    val nextCursor: String?,
)

data class ContextSentence(
    val id: Long,
    val itemType: String,
    val itemId: Long,
    val sentence: String,
    val translation: String,
    val highlight: String,
    val highlightStart: Int,
    val highlightEnd: Int,
    val audioUrl: String,
    val contextType: String = "text",
    val videos: List<ContextVideo> = emptyList(),
)

data class ContextVideo(
    val id: Long,
    val videoUrl: String,
    val subtitleUrl: String,
    val durationMs: Long,
)

internal fun sanitizeContextVideos(videos: List<ContextVideo>): List<ContextVideo> =
    videos.filter { it.id > 0L && it.videoUrl.isNotEmpty() && it.durationMs > 0L }

internal fun parseContextVideos(data: JSONObject): List<ContextVideo> {
    val videos = mutableListOf<ContextVideo>()
    data.optJSONArray("videos")?.let { array ->
        for (index in 0 until array.length()) {
            val item = array.optJSONObject(index) ?: continue
            val video = ContextVideo(
                id = item.optLong("id", 0L),
                videoUrl = item.optString("videoUrl", "").trim(),
                subtitleUrl = item.optString("subtitleUrl", "").trim(),
                durationMs = item.optLong("durationMs", 0L),
            )
            videos += video
        }
    }
    return sanitizeContextVideos(videos)
}

data class ContextsData(
    val word: String,
    val contexts: List<ContextSentence>,
)

data class FavoriteStatus(
    val favorited: Boolean,
    val itemId: Long,
    val level: Int = 0,
)

internal fun buildFavoriteActionBody(
    eventId: String,
    action: String,
    source: String,
    metadata: JSONObject = JSONObject(),
): JSONObject = JSONObject()
    .put("eventId", eventId)
    .put("action", action)
    .put("source", source)
    .put("metadata", metadata)

data class PhraseDetailData(
    val itemType: String,
    val itemId: Long,
    val phrase: String,
    val audioUrl: String,
    val meanings: List<ChineseMeaning>,
    val contexts: List<ContextSentence>,
)

data class NoteData(
    val itemType: String,
    val text: String,
    val note: String,
    val updatedAt: String,
)

object WordApi {

    suspend fun lookup(word: String): Result<LookupData> = withContext(Dispatchers.IO) {
        runCatching {
            val json = postJson(WordServiceConfig.apiBaseUrl + "/lookup", JSONObject().put("word", word))
            val data = json.optJSONObject("data") ?: return@runCatching LookupData(word, "", emptyList(), emptyList())

            val phonetic = data.optString("phonetic", "")
            val prons = mutableListOf<Pronunciation>()
            data.optJSONArray("pronunciations")?.let { arr ->
                for (i in 0 until arr.length()) {
                    val obj = arr.optJSONObject(i) ?: continue
                    prons += Pronunciation(
                        accent = obj.optString("accent", "").trim(),
                        text = obj.optString("text", "").trim(),
                        audioUrl = obj.optString("audioUrl", "").trim(),
                    )
                }
            }

            val defs = mutableListOf<Definition>()
            data.optJSONArray("meanings")?.let { arr ->
                for (i in 0 until arr.length()) {
                    val m = arr.optJSONObject(i) ?: continue
                    val pos = m.optString("partOfSpeech", "").trim()
                    val defArr = m.optJSONArray("definitions") ?: continue
                    for (j in 0 until defArr.length()) {
                        val d = defArr.optJSONObject(j) ?: continue
                        val text = d.optString("definition", "").trim()
                        if (text.isNotEmpty()) defs += Definition(pos = pos, text = text)
                    }
                }
            }

            LookupData(
                word = data.optString("word", word),
                phonetic = phonetic,
                pronunciations = prons.filter { it.audioUrl.isNotEmpty() || it.text.isNotEmpty() || it.accent.isNotEmpty() },
                definitions = defs,
                pos = data.optString("pos", "").trim(),
                collins = data.optInt("collins", 0),
                oxford = data.optInt("oxford", 0),
                tags = parseStringArray(data, "tags"),
                exchange = data.optString("exchange", "").trim(),
            )
        }
    }

    suspend fun meaning(word: String): Result<MeaningData> = withContext(Dispatchers.IO) {
        runCatching {
            val json = postJson(
                WordServiceConfig.apiBaseUrl + "/meaning",
                JSONObject().put("word", word),
                readTimeoutMs = LLM_READ_TIMEOUT_MS,
            )
            val data = json.optJSONObject("data") ?: return@runCatching MeaningData(word, emptyList())
            parseMeaningData(data, word)
        }
    }

    suspend fun confusableWords(word: String): Result<ConfusableWordsData?> = withContext(Dispatchers.IO) {
        runCatching {
            val normalized = word.trim().lowercase(Locale.ROOT)
            val path = "/groups/by-word?word=" +
                java.net.URLEncoder.encode(normalized, "UTF-8")
            val data = getPreferredWordJson(path).optJSONObject("data") ?: return@runCatching null
            parseConfusableWordsData(data)
        }
    }

    suspend fun associateWords(
        word: String,
        meaningHint: String,
        spellingHint: String,
    ): Result<WordAssociationData> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("word", word.trim().lowercase(Locale.ROOT))
                .put("meaningHint", meaningHint.trim())
                .put("spellingHint", spellingHint.trim())
            val data = postJson(
                WordServiceConfig.apiBaseUrl + "/associations",
                body,
                readTimeoutMs = LLM_READ_TIMEOUT_MS,
            ).optJSONObject("data")
            val items = mutableListOf<WordAssociationItem>()
            data?.optJSONArray("items")?.let { array ->
                for (i in 0 until array.length()) {
                    val item = array.optJSONObject(i) ?: continue
                    items += WordAssociationItem(
                        word = item.optString("word", "").trim(),
                        topMeaningText = item.optString("topMeaningText", "").trim(),
                        similarity = item.optDouble("similarity", 0.0),
                    )
                }
            }
            WordAssociationData(sanitizeWordAssociations(items))
        }
    }

    suspend fun addConfusableWord(
        word: String,
        candidateWord: String,
    ): Result<ConfusableWordsData> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("word", word.trim().lowercase(Locale.ROOT))
                .put("candidateWord", candidateWord.trim().lowercase(Locale.ROOT))
            val data = postPreferredWordJson(
                "/groups/members",
                body,
                readTimeoutMs = WORD_INITIALIZATION_READ_TIMEOUT_MS,
            ).optJSONObject("data") ?: throw IOException("add confusable word response missing data")
            val parsed = parseConfusableWordsData(data)
            if (parsed.groupId <= 0L || parsed.members.isEmpty()) {
                throw IOException("add confusable word response invalid group")
            }
            parsed
        }
    }

    suspend fun phrase(phrase: String): Result<MeaningData> = withContext(Dispatchers.IO) {
        runCatching {
            val json = postJson(
                WordServiceConfig.apiBaseUrl + "/phrase",
                JSONObject().put("phrase", phrase),
                readTimeoutMs = LLM_READ_TIMEOUT_MS,
            )
            val data = json.optJSONObject("data") ?: return@runCatching MeaningData(phrase, emptyList())
            parseMeaningData(data, phrase)
        }
    }

    suspend fun phraseFavoriteStatus(phrase: String): Result<FavoriteStatus> = withContext(Dispatchers.IO) {
        runCatching {
            val urlStr = WordServiceConfig.origin + "/api/v1/phrase/favorite/status?phrase=" +
                java.net.URLEncoder.encode(phrase, "UTF-8")
            val json = getJson(urlStr)
            val data = json.optJSONObject("data")
            FavoriteStatus(
                favorited = data?.optBoolean("favorited", false) ?: false,
                itemId = data?.optLong("itemId", 0L) ?: 0L,
                level = data?.optInt("level", 0) ?: 0,
            )
        }
    }

    suspend fun favoritePhrase(
        phrase: String,
        meaning: MeaningData,
    ): Result<Long> = withContext(Dispatchers.IO) {
        runCatching {
            val meanings = org.json.JSONArray()
            meaning.meanings.forEach { item ->
                meanings.put(
                    JSONObject()
                        .put("partOfSpeech", item.pos)
                        .put("meaning", item.meaning)
                )
            }
            val body = JSONObject()
                .put("phrase", phrase)
                .put(
                    "meaning",
                    JSONObject()
                        .put("word", meaning.word.ifBlank { phrase })
                        .put("meanings", meanings)
                )
            val response = postJson(
                WordServiceConfig.origin + "/api/v1/phrase/favorite",
                body,
                readTimeoutMs = CONTEXT_TASK_READ_TIMEOUT_MS,
            )
            val itemId = response.optJSONObject("data")?.optLong("itemId", 0L) ?: 0L
            if (itemId <= 0L) throw IOException("phrase favorite response missing itemId")
            itemId
        }
    }

    suspend fun phraseDetail(itemId: Long): Result<PhraseDetailData> = withContext(Dispatchers.IO) {
        runCatching {
            val json = getJson(WordServiceConfig.origin + "/api/v1/phrase/$itemId")
            val data = json.optJSONObject("data") ?: throw IOException("phrase detail response missing data")
            val meanings = mutableListOf<ChineseMeaning>()
            data.optJSONArray("meanings")?.let { array ->
                for (i in 0 until array.length()) {
                    val item = array.optJSONObject(i) ?: continue
                    val text = item.optString("meaning", "").trim()
                    if (text.isNotEmpty()) {
                        meanings += ChineseMeaning(
                            pos = item.optString("partOfSpeech", "").trim(),
                            meaning = text,
                        )
                    }
                }
            }
            PhraseDetailData(
                itemType = data.optString("itemType", "phrase"),
                itemId = data.optLong("itemId", itemId),
                phrase = data.optString("phrase", "").trim(),
                audioUrl = data.optString("audioUrl", "").trim(),
                meanings = meanings,
                contexts = parseContexts(data),
            )
        }
    }

    suspend fun getNote(itemType: String, text: String): Result<NoteData> = withContext(Dispatchers.IO) {
        runCatching {
            val url = WordServiceConfig.origin + "/api/v1/notes?itemType=" +
                java.net.URLEncoder.encode(itemType, "UTF-8") + "&text=" +
                java.net.URLEncoder.encode(text, "UTF-8")
            parseNoteData(getJson(url), itemType, text)
        }
    }

    suspend fun saveNote(itemType: String, text: String, note: String): Result<NoteData> =
        withContext(Dispatchers.IO) {
            runCatching {
                val body = JSONObject()
                    .put("itemType", itemType)
                    .put("text", text)
                    .put("note", note)
                parseNoteData(
                    putJson(WordServiceConfig.origin + "/api/v1/notes", body),
                    itemType,
                    text,
                )
            }
        }

    private fun parseNoteData(root: JSONObject, fallbackType: String, fallbackText: String): NoteData {
        val data = root.optJSONObject("data") ?: JSONObject()
        return NoteData(
            itemType = data.optString("itemType", fallbackType),
            text = data.optString("text", fallbackText),
            note = data.optString("note", ""),
            updatedAt = data.optString("updatedAt", ""),
        )
    }

    private fun parseMeaningData(data: JSONObject, fallback: String): MeaningData {
        val list = mutableListOf<ChineseMeaning>()
        data.optJSONArray("meanings")?.let { arr ->
            for (i in 0 until arr.length()) {
                val obj = arr.optJSONObject(i) ?: continue
                val meaning = obj.optString("meaning", "").trim()
                if (meaning.isEmpty()) continue
                list += ChineseMeaning(
                    pos = obj.optString("partOfSpeech", "").trim(),
                    meaning = meaning,
                )
            }
        }
        return MeaningData(
            word = data.optString("word", fallback),
            meanings = list,
            pos = data.optString("pos", "").trim(),
            collins = data.optInt("collins", 0),
            oxford = data.optInt("oxford", 0),
            tags = parseStringArray(data, "tags"),
        )
    }

    private fun parseStringArray(data: JSONObject, key: String): List<String> {
        val array = data.optJSONArray(key) ?: return emptyList()
        val result = mutableListOf<String>()
        for (i in 0 until array.length()) {
            val text = array.optString(i, "").trim()
            if (text.isNotEmpty()) result += text
        }
        return result
    }

    suspend fun explain(
        word: String,
        context: String,
        wordStart: Int,
        wordEnd: Int,
    ): Result<ExplainData> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("word", word)
                .put("context", context)
                .put("wordStart", wordStart)
                .put("wordEnd", wordEnd)
            val json = postJson(
                WordServiceConfig.apiBaseUrl + "/explain",
                body,
                readTimeoutMs = LLM_READ_TIMEOUT_MS,
            )
            val data = json.optJSONObject("data") ?: return@runCatching ExplainData(word, "")
            ExplainData(
                word = data.optString("word", word),
                explanation = data.optString("explanation", "").trim(),
            )
        }
    }

    /**
     * 句子语法分析。走 SSE 协议：服务端会持续发心跳注释帧防网关超时，
     * 结果通过 event: result 一次性下发；最后收到 data: [DONE] 或 event: error。
     * 该请求耗时通常 20~60s，因此专门放宽 readTimeout 到 90s（心跳间隔 10s，理应不会撞到）。
     */
    suspend fun analyzeSentence(sentence: String): Result<SentenceAnalysis> = withContext(Dispatchers.IO) {
        val urlStr = WordServiceConfig.apiBaseUrl + "/sentence/analyze"
        val t0 = System.currentTimeMillis()
        val url = URL(urlStr)
        val conn = url.openConnection() as HttpURLConnection
        var heartbeatCount = 0
        try {
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.doInput = true
            conn.connectTimeout = 8_000
            conn.readTimeout = 90_000
            conn.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            conn.setRequestProperty("Accept", "text/event-stream")
            conn.setRequestProperty("Connection", "close")
            val body = JSONObject().put("sentence", sentence).toString()
            conn.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            val code = conn.responseCode
            if (code !in 200..299) {
                val errText = runCatching {
                    conn.errorStream?.use { it.reader(Charsets.UTF_8).readText() }
                }.getOrNull().orEmpty()
                Log.w(
                    TAG,
                    "analyze sse non-2xx url=$urlStr code=$code msg=${conn.responseMessage} elapsed=${System.currentTimeMillis() - t0}ms body=${errText.take(500)}",
                )
                return@withContext Result.failure<SentenceAnalysis>(IOException("HTTP $code"))
            }
            Log.d(TAG, "analyze sse connected url=$urlStr code=$code")
            val reader = BufferedReader(InputStreamReader(conn.inputStream, Charsets.UTF_8))
            var currentEvent = "message"
            var resultJson: String? = null
            var errorMsg: String? = null
            reader.use { r ->
                while (true) {
                    val line = r.readLine() ?: break
                    if (line.isEmpty()) {
                        currentEvent = "message"
                        continue
                    }
                    // 服务端心跳是 SSE 注释帧（以 ':' 开头），忽略即可
                    if (line.startsWith(":")) {
                        heartbeatCount++
                        continue
                    }
                    when {
                        line.startsWith("event:") -> {
                            currentEvent = line.substring(6).trim()
                        }
                        line.startsWith("data:") -> {
                            val data = line.substring(5).trim()
                            when (currentEvent) {
                                "error" -> {
                                    errorMsg = runCatching { JSONObject(data).optString("msg", "") }
                                        .getOrDefault("")
                                        .ifEmpty { "analyze failed" }
                                    Log.w(TAG, "analyze sse error frame sentence=${sentence.take(80)} msg=$errorMsg")
                                }
                                "result" -> {
                                    resultJson = data
                                    Log.d(
                                        TAG,
                                        "analyze sse result received url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms heartbeats=$heartbeatCount resp_len=${data.length}",
                                    )
                                }
                                else -> {
                                    if (data == "[DONE]") {
                                        Log.d(
                                            TAG,
                                            "analyze sse done url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms heartbeats=$heartbeatCount has_result=${resultJson != null} has_error=${errorMsg != null}",
                                        )
                                        break
                                    }
                                }
                            }
                        }
                        else -> {}
                    }
                }
            }
            val err = errorMsg
            if (err != null) {
                return@withContext Result.failure<SentenceAnalysis>(IOException(err))
            }
            val raw = resultJson
                ?: return@withContext Result.failure<SentenceAnalysis>(IOException("analyze: no result frame"))
            val parsed = parseSentenceAnalysisJson(raw, fallbackSentence = sentence)
            Result.success(parsed)
        } catch (t: Throwable) {
            Log.w(
                TAG,
                "analyze sse failed url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms heartbeats=$heartbeatCount err_type=${t.javaClass.name} err_msg=${t.message}",
                t,
            )
            Result.failure(t)
        } finally {
            runCatching { conn.disconnect() }
        }
    }

    suspend fun sentenceFavoriteStatus(sentence: String): Result<SentenceFavoriteStatus> =
        withContext(Dispatchers.IO) {
            runCatching {
                val url = WordServiceConfig.origin + "/api/v1/sentences/favorites/status?sentence=" +
                    java.net.URLEncoder.encode(sentence, "UTF-8")
                val data = getJson(url).optJSONObject("data")
                SentenceFavoriteStatus(
                    favorited = data?.optBoolean("favorited", false) ?: false,
                    id = data?.optLong("id", 0L) ?: 0L,
                )
            }
        }

    suspend fun favoriteSentence(
        sentence: String,
        translation: String,
    ): Result<SentenceFavorite> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("sentence", sentence)
                .put("translation", translation)
            val data = postJson(
                WordServiceConfig.origin + "/api/v1/sentences/favorites",
                body,
            ).optJSONObject("data") ?: throw IOException("sentence favorite response missing data")
            parseSentenceFavorite(data)
                ?: throw IOException("sentence favorite response invalid")
        }
    }

    suspend fun sentenceFavorites(
        tagIds: Set<Long> = emptySet(),
    ): Result<SentenceFavoriteListData> = withContext(Dispatchers.IO) {
        runCatching {
            val data = getJson(buildSentenceFavoritesUrl(tagIds)).optJSONObject("data")
            val items = mutableListOf<SentenceFavorite>()
            data?.optJSONArray("items")?.let { array ->
                for (i in 0 until array.length()) {
                    val item = array.optJSONObject(i) ?: continue
                    parseSentenceFavorite(item)?.let(items::add)
                }
            }
            SentenceFavoriteListData(
                items = sanitizeSentenceFavorites(items),
                total = data?.optInt("total", items.size) ?: items.size,
            )
        }
    }

    suspend fun sentenceTags(): Result<List<SentenceTag>> = withContext(Dispatchers.IO) {
        runCatching {
            val data = getJson(
                WordServiceConfig.origin + "/api/v1/sentence-tags",
            ).optJSONObject("data")
            val items = mutableListOf<SentenceTag>()
            data?.optJSONArray("items")?.let { array ->
                for (i in 0 until array.length()) {
                    parseSentenceTag(array.optJSONObject(i))?.let(items::add)
                }
            }
            items
        }
    }

    suspend fun createSentenceTag(
        name: String,
        color: String,
    ): Result<SentenceTag> = withContext(Dispatchers.IO) {
        runCatching {
            val data = postJson(
                WordServiceConfig.origin + "/api/v1/sentence-tags",
                JSONObject().put("name", name).put("color", color),
            ).optJSONObject("data") ?: throw IOException("sentence tag response missing data")
            parseSentenceTag(data) ?: throw IOException("sentence tag response invalid")
        }
    }

    suspend fun replaceSentenceFavoriteTags(
        sentenceId: Long,
        tagIds: Set<Long>,
        note: String,
    ): Result<SentenceFavorite> = withContext(Dispatchers.IO) {
        runCatching {
            val tagIDArray = org.json.JSONArray()
            tagIds.filter { it > 0L }.sorted().forEach(tagIDArray::put)
            val data = putJson(
                WordServiceConfig.origin + "/api/v1/sentences/favorites/$sentenceId/tags",
                JSONObject()
                    .put("tagIds", tagIDArray)
                    .put("note", note),
            ).optJSONObject("data") ?: throw IOException("sentence tags response missing data")
            parseSentenceFavorite(data)
                ?: throw IOException("sentence tags response invalid")
        }
    }

    suspend fun sentenceFavoriteDetail(id: Long): Result<SentenceFavorite> = withContext(Dispatchers.IO) {
        runCatching {
            val data = getJson(
                WordServiceConfig.origin + "/api/v1/sentences/favorites/$id",
            ).optJSONObject("data") ?: throw IOException("sentence favorite detail missing data")
            parseSentenceFavorite(data)
                ?: throw IOException("sentence favorite detail invalid")
        }
    }

    private fun parseSentenceFavorite(data: JSONObject): SentenceFavorite? {
        val tags = mutableListOf<SentenceFavoriteTag>()
        data.optJSONArray("tags")?.let { array ->
            for (i in 0 until array.length()) {
                val item = array.optJSONObject(i) ?: continue
                val id = item.optLong("id", 0L)
                val name = item.optString("name", "").trim()
                val color = item.optString("color", "").trim().uppercase(Locale.US)
                if (id > 0L && name.isNotEmpty() && isSentenceTagColor(color)) {
                    tags += SentenceFavoriteTag(id, name, color)
                }
            }
        }
        val item = SentenceFavorite(
            id = data.optLong("id", 0L),
            sentence = data.optString("sentence", "").trim(),
            translation = data.optString("translation", "").trim(),
            createdAt = data.optString("createdAt", "").trim(),
            note = data.optString("note", "").trim(),
            tags = tags,
        )
        return item.takeIf { it.id > 0L && it.sentence.isNotEmpty() }
    }

    private fun parseSentenceTag(data: JSONObject?): SentenceTag? {
        data ?: return null
        val id = data.optLong("id", 0L)
        val name = data.optString("name", "").trim()
        val color = data.optString("color", "").trim().uppercase(Locale.US)
        if (id <= 0L || name.isEmpty() || !isSentenceTagColor(color)) return null
        return SentenceTag(
            id = id,
            name = name,
            color = color,
            createdAt = data.optString("createdAt", "").trim(),
        )
    }

    private fun isSentenceTagColor(color: String): Boolean =
        color.matches(Regex("^#[0-9A-F]{6}$"))

    /**
     * 解析后端 SentenceAnalysis JSON。允许外层是 {code,data:{...}}（旧格式）或直接是 SentenceAnalysis 结构（新 SSE result 帧格式）。
     */
    private fun parseSentenceAnalysisJson(raw: String, fallbackSentence: String): SentenceAnalysis {
        val root = JSONObject(raw)
        val data = root.optJSONObject("data") ?: root
        val chunks = mutableListOf<SentenceChunk>()
        data.optJSONArray("chunks")?.let { arr ->
            for (i in 0 until arr.length()) {
                val obj = arr.optJSONObject(i) ?: continue
                val text = obj.optString("text", "")
                if (text.isEmpty()) continue
                chunks += SentenceChunk(
                    text = text,
                    type = obj.optString("type", "other").trim().ifEmpty { "other" },
                    role = obj.optString("role", "").trim(),
                    translation = obj.optString("translation", "").trim(),
                )
            }
        }
        return SentenceAnalysis(
            sentence = data.optString("sentence", fallbackSentence),
            structure = data.optString("structure", "").trim(),
            chunks = chunks,
        )
    }

    /**
     * SSE 流式翻译。onDelta/onDone/onError 均在 IO 线程回调。任一失败或完成后自动断开连接。
     * 该方法内部处理连接、读流、事件解析；不使用 postJson，因为需要长连接 stream。
     */
    suspend fun translateSentenceStream(
        sentence: String,
        onDelta: (String) -> Unit,
        onDone: () -> Unit,
        onError: (String) -> Unit,
    ) = withContext(Dispatchers.IO) {
        val urlStr = WordServiceConfig.apiBaseUrl + "/sentence/translate"
        val t0 = System.currentTimeMillis()
        val url = URL(urlStr)
        val conn = url.openConnection() as HttpURLConnection
        var deltaCount = 0
        var errorEmitted = false
        try {
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.doInput = true
            conn.connectTimeout = 8_000
            conn.readTimeout = LLM_READ_TIMEOUT_MS
            conn.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            conn.setRequestProperty("Accept", "text/event-stream")
            conn.setRequestProperty("Connection", "close")
            val body = JSONObject().put("sentence", sentence).toString()
            conn.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }
            val code = conn.responseCode
            if (code !in 200..299) {
                val errText = runCatching {
                    conn.errorStream?.use { it.reader(Charsets.UTF_8).readText() }
                }.getOrNull().orEmpty()
                Log.w(
                    TAG,
                    "translate stream non-2xx url=$urlStr code=$code msg=${conn.responseMessage} elapsed=${System.currentTimeMillis() - t0}ms body=${errText.take(500)}",
                )
                errorEmitted = true
                onError("HTTP $code")
                return@withContext
            }
            Log.d(TAG, "translate stream connected url=$urlStr code=$code")
            val reader = BufferedReader(InputStreamReader(conn.inputStream, Charsets.UTF_8))
            var currentEvent = "message"
            reader.use { r ->
                while (true) {
                    val line = r.readLine() ?: break
                    if (line.isEmpty()) {
                        currentEvent = "message"
                        continue
                    }
                    when {
                        line.startsWith("event:") -> {
                            currentEvent = line.substring(6).trim()
                        }
                        line.startsWith("data:") -> {
                            val data = line.substring(5).trim()
                            if (currentEvent == "error") {
                                val msg = runCatching { JSONObject(data).optString("msg", "") }
                                    .getOrDefault("")
                                    .ifEmpty { "translate failed" }
                                Log.w(TAG, "translate stream error frame sentence=${sentence.take(80)} msg=$msg")
                                errorEmitted = true
                                onError(msg)
                                return@withContext
                            }
                            if (data == "[DONE]") {
                                Log.d(
                                    TAG,
                                    "translate stream done url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms delta_count=$deltaCount",
                                )
                                onDone()
                                return@withContext
                            }
                            val delta = runCatching { JSONObject(data).optString("delta", "") }
                                .getOrDefault("")
                            if (delta.isNotEmpty()) {
                                if (deltaCount == 0) {
                                    Log.d(
                                        TAG,
                                        "translate stream first delta url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms",
                                    )
                                }
                                deltaCount++
                                onDelta(delta)
                            }
                        }
                        // 其它行（如 : 注释、id: ...）忽略
                        else -> {}
                    }
                }
            }
            // 未收到 [DONE] 就 EOF：视作已结束
            Log.d(
                TAG,
                "translate stream eof url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms delta_count=$deltaCount",
            )
            onDone()
        } catch (t: Throwable) {
            Log.w(
                TAG,
                "translate stream failed url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms delta_count=$deltaCount err_type=${t.javaClass.name} err_msg=${t.message}",
                t,
            )
            if (!errorEmitted) {
                onError(t.message ?: t.javaClass.simpleName)
            }
        } finally {
            runCatching { conn.disconnect() }
        }
    }

    fun audioAbsoluteUrl(rawUrl: String): String {
        val trimmed = rawUrl.trim()
        if (trimmed.isEmpty()) return trimmed
        if (trimmed.startsWith("http://", ignoreCase = true) || trimmed.startsWith("https://", ignoreCase = true)) {
            return trimmed
        }
        val path = if (trimmed.startsWith("/")) trimmed else "/$trimmed"
        return WordServiceConfig.origin + path
    }

    fun mediaAbsoluteUrl(rawUrl: String): String = audioAbsoluteUrl(rawUrl)

    suspend fun favorites(q: String? = null): Result<FavoriteListData> = withContext(Dispatchers.IO) {
        runCatching {
            val url = buildString {
                append(WordServiceConfig.apiBaseUrl)
                append("/favorites")
                val trimmed = q?.trim().orEmpty()
                if (trimmed.isNotEmpty()) {
                    append("?q=")
                    append(java.net.URLEncoder.encode(trimmed, "UTF-8"))
                }
            }
            val json = getJson(url)
            val data = json.optJSONObject("data") ?: return@runCatching FavoriteListData(emptyList(), 0)
            val items = mutableListOf<FavoriteItem>()
            data.optJSONArray("items")?.let { arr ->
                for (i in 0 until arr.length()) {
                    val obj = arr.optJSONObject(i) ?: continue
                    parseFavoriteItem(obj)?.let(items::add)
                }
            }
            FavoriteListData(
                items = items,
                total = data.optInt("total", items.size),
            )
        }
    }

    suspend fun favoriteGroups(): Result<FavoriteGroupListData> = withContext(Dispatchers.IO) {
        runCatching {
            val json = getJson(WordServiceConfig.apiBaseUrl + "/favorite-groups")
            val data = json.optJSONObject("data") ?: return@runCatching FavoriteGroupListData(emptyList(), 0)
            val groups = mutableListOf<FavoriteGroupSummary>()
            data.optJSONArray("groups")?.let { arr ->
                for (i in 0 until arr.length()) {
                    val obj = arr.optJSONObject(i) ?: continue
                    val id = obj.optString("id", "").trim()
                    val rangeStart = obj.optString("rangeStart", "").trim()
                    val rangeEnd = obj.optString("rangeEnd", "").trim()
                    if (id.isEmpty() || rangeStart.isEmpty() || rangeEnd.isEmpty()) continue
                    groups += FavoriteGroupSummary(
                        id = id,
                        type = obj.optString("type", "").trim(),
                        rangeStart = rangeStart,
                        rangeEnd = rangeEnd,
                        displayStart = obj.optString("displayStart", rangeStart).trim(),
                        displayEnd = obj.optString("displayEnd", rangeEnd).trim(),
                        count = obj.optInt("count", 0),
                    )
                }
            }
            FavoriteGroupListData(groups = groups, total = data.optInt("total", 0))
        }
    }

    suspend fun favoritesPage(
        startAt: String,
        endAt: String,
        cursor: String? = null,
        limit: Int = 50,
    ): Result<FavoritePageData> = withContext(Dispatchers.IO) {
        runCatching {
            val encode = { value: String -> java.net.URLEncoder.encode(value, "UTF-8") }
            val url = buildString {
                append(WordServiceConfig.apiBaseUrl)
                append("/favorites?startAt=")
                append(encode(startAt))
                append("&endAt=")
                append(encode(endAt))
                append("&limit=")
                append(limit)
                cursor?.takeIf { it.isNotBlank() }?.let {
                    append("&cursor=")
                    append(encode(it))
                }
            }
            val json = getJson(url)
            val data = json.optJSONObject("data") ?: return@runCatching FavoritePageData(emptyList(), null)
            val items = mutableListOf<FavoriteItem>()
            data.optJSONArray("items")?.let { arr ->
                for (i in 0 until arr.length()) {
                    val obj = arr.optJSONObject(i) ?: continue
                    parseFavoriteItem(obj)?.let(items::add)
                }
            }
            FavoritePageData(
                items = items,
                nextCursor = data.optString("nextCursor", "").trim().ifEmpty { null },
            )
        }
    }

    private fun parseFavoriteItem(obj: JSONObject): FavoriteItem? {
        val text = obj.optString("text", "").trim()
            .ifEmpty { obj.optString("word", "").trim() }
        if (text.isEmpty()) return null
        return FavoriteItem(
            itemType = obj.optString("itemType", "word").trim().ifEmpty { "word" },
            itemId = obj.optLong("itemId", 0L),
            text = text,
            level = obj.optInt("level", 0),
            createdAt = obj.optString("createdAt", "").trim(),
            topMeaningPos = obj.optString("topMeaningPos", "").trim(),
            topMeaningText = obj.optString("topMeaningText", "").trim(),
        )
    }

    suspend fun deleteFavorite(itemType: String, itemId: Long): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val normalizedType = itemType.trim().lowercase()
            if (normalizedType != "word" && normalizedType != "phrase") {
                throw IllegalArgumentException("invalid favorite item type")
            }
            if (itemId <= 0L) {
                throw IllegalArgumentException("invalid favorite item id")
            }
            deleteJson(
                WordServiceConfig.origin + "/api/v1/favorites/$normalizedType/$itemId"
            )
            Unit
        }
    }

    suspend fun contexts(word: String): Result<ContextsData> = withContext(Dispatchers.IO) {
        runCatching {
            val json = postJson(WordServiceConfig.apiBaseUrl + "/contexts", JSONObject().put("word", word))
            val data = json.optJSONObject("data") ?: return@runCatching ContextsData(word, emptyList())
            ContextsData(
                word = data.optString("word", word),
                contexts = parseContexts(data),
            )
        }
    }

    private fun parseContexts(data: JSONObject): List<ContextSentence> {
        val list = mutableListOf<ContextSentence>()
        data.optJSONArray("contexts")?.let { arr ->
            for (i in 0 until arr.length()) {
                val obj = arr.optJSONObject(i) ?: continue
                list += ContextSentence(
                    id = obj.optLong("id", 0L),
                    itemType = obj.optString("itemType", "word").trim(),
                    itemId = obj.optLong("itemId", 0L),
                    sentence = obj.optString("sentence", "").trim(),
                    translation = obj.optString("translation", "").trim(),
                    highlight = obj.optString("highlight", "").trim(),
                    highlightStart = obj.optInt("highlightStart", -1),
                    highlightEnd = obj.optInt("highlightEnd", -1),
                    audioUrl = obj.optString("audioUrl", "").trim(),
                    contextType = obj.optString("contextType", "text").trim().ifEmpty { "text" },
                    videos = parseContextVideos(obj),
                )
            }
        }
        return list.filter { it.sentence.isNotEmpty() }
    }

    suspend fun favoriteStatus(word: String): Result<FavoriteStatus> = withContext(Dispatchers.IO) {
        runCatching {
            val urlStr = WordServiceConfig.apiBaseUrl + "/favorite/status?word=" +
                java.net.URLEncoder.encode(word, "UTF-8")
            val json = getJson(urlStr)
            val data = json.optJSONObject("data")
            FavoriteStatus(
                favorited = data?.optBoolean("favorited", false) ?: false,
                itemId = data?.optLong("itemId", 0L) ?: 0L,
                level = data?.optInt("level", 0) ?: 0,
            )
        }
    }

    suspend fun recordFavoriteAction(
        itemType: String,
        itemId: Long,
        eventId: String,
        action: String,
        source: String,
        metadata: JSONObject = JSONObject(),
    ): Result<Int> = withContext(Dispatchers.IO) {
        runCatching {
            val normalizedType = itemType.trim().lowercase(Locale.ROOT)
            require(normalizedType == "word" || normalizedType == "phrase")
            require(itemId > 0L && eventId.isNotBlank())
            val response = postJson(
                WordServiceConfig.origin + "/api/v1/favorites/$normalizedType/$itemId/actions",
                buildFavoriteActionBody(eventId, action, source, metadata),
            )
            response.optJSONObject("data")?.optInt("level", 0) ?: 0
        }
    }

    suspend fun favorite(
        word: String,
        english: LookupData?,
        chinese: MeaningData?,
    ): Result<Long> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
            body.put("word", word)

            val wordData = JSONObject()
            wordData.put("word", word)
            wordData.put("phonetic", english?.phonetic ?: "")

            val pronArr = org.json.JSONArray()
            english?.pronunciations?.forEach { p ->
                pronArr.put(
                    JSONObject()
                        .put("accent", p.accent)
                        .put("text", p.text)
                        .put("audioUrl", p.audioUrl)
                )
            }
            wordData.put("pronunciations", pronArr)

            val enMeaningsArr = org.json.JSONArray()
            english?.definitions?.let { defs ->
                val grouped = linkedMapOf<String, MutableList<String>>()
                defs.forEach { d ->
                    grouped.getOrPut(d.pos) { mutableListOf() }.add(d.text)
                }
                grouped.forEach { (pos, texts) ->
                    val defsArr = org.json.JSONArray()
                    texts.forEach { t ->
                        defsArr.put(
                            JSONObject()
                                .put("definition", t)
                                .put("example", "")
                                .put("synonyms", org.json.JSONArray())
                                .put("antonyms", org.json.JSONArray())
                        )
                    }
                    enMeaningsArr.put(
                        JSONObject()
                            .put("partOfSpeech", pos)
                            .put("definitions", defsArr)
                    )
                }
            }
            wordData.put("meanings", enMeaningsArr)
            body.put("wordData", wordData)

            if (chinese != null && chinese.meanings.isNotEmpty()) {
                val meaningObj = JSONObject()
                meaningObj.put("word", chinese.word.ifBlank { word })
                val zhArr = org.json.JSONArray()
                chinese.meanings.forEach { m ->
                    zhArr.put(
                        JSONObject()
                            .put("partOfSpeech", m.pos)
                            .put("meaning", m.meaning)
                    )
                }
                meaningObj.put("meanings", zhArr)
                body.put("meaning", meaningObj)
            }

            val response = postJson(WordServiceConfig.apiBaseUrl + "/favorite", body)
            val itemId = response.optJSONObject("data")?.optLong("itemId", 0L) ?: 0L
            if (itemId <= 0L) {
                throw IOException("favorite response missing itemId")
            }
            itemId
        }
    }

    suspend fun createContextTask(
        itemType: String,
        itemId: Long,
        paragraph: String,
        selectionStart: Int,
        selectionEnd: Int,
    ): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("itemType", itemType)
                .put("itemId", itemId)
                .put("paragraph", paragraph)
                .put("selectionStart", selectionStart)
                .put("selectionEnd", selectionEnd)
                .put("source", "android_reader")
            postJson(
                WordServiceConfig.origin + "/api/v1/contexts/tasks",
                body,
                readTimeoutMs = CONTEXT_TASK_READ_TIMEOUT_MS,
            )
            Unit
        }
    }

    suspend fun resetFavoriteLearning(itemType: String, itemId: Long): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val url = WordServiceConfig.origin + "/api/v1/favorites/" + itemType + "/" + itemId + "/learning/reset"
            postJson(url, JSONObject())
            Unit
        }
    }

    private fun deleteJson(urlStr: String): JSONObject {
        val t0 = System.currentTimeMillis()
        val conn = URL(urlStr).openConnection() as HttpURLConnection
        try {
            conn.requestMethod = "DELETE"
            conn.doInput = true
            conn.doOutput = false
            conn.connectTimeout = 8000
            conn.readTimeout = DEFAULT_READ_TIMEOUT_MS
            conn.setRequestProperty("Accept", "application/json")
            conn.setRequestProperty("Connection", "close")
            val code = conn.responseCode
            val text = if (code in 200..299) {
                conn.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                conn.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                Log.w(
                    TAG,
                    "deleteJson non-2xx url=$urlStr code=$code msg=${conn.responseMessage} elapsed=${System.currentTimeMillis() - t0}ms body=${text.take(500)}",
                )
                throw IOException("HTTP $code ${runCatching { JSONObject(text).optString("msg") }.getOrDefault("")}")
            }
            Log.d(
                TAG,
                "deleteJson ok url=$urlStr code=$code elapsed=${System.currentTimeMillis() - t0}ms",
            )
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        } catch (t: Throwable) {
            Log.w(
                TAG,
                "deleteJson failed url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms err_type=${t.javaClass.name} err_msg=${t.message}",
                t,
            )
            throw t
        } finally {
            conn.disconnect()
        }
    }

    private fun getPreferredWordJson(
        path: String,
        readTimeoutMs: Int = DEFAULT_READ_TIMEOUT_MS,
    ): JSONObject {
        val baseUrls = WordServiceConfig.preferredApiBaseUrls()
        var lastError: IOException? = null
        for ((index, baseUrl) in baseUrls.withIndex()) {
            try {
                return getJson(
                    baseUrl + path,
                    readTimeoutMs = readTimeoutMs,
                    connectTimeoutMs = if (index == baseUrls.lastIndex) {
                        DEFAULT_CONNECT_TIMEOUT_MS
                    } else {
                        LOCAL_CONNECT_TIMEOUT_MS
                    },
                )
            } catch (e: ConnectionOpenException) {
                lastError = e
                if (index == baseUrls.lastIndex) throw e
                Log.w(TAG, "local word service unavailable url=${e.url}; falling back to public service")
            }
        }
        throw lastError ?: IOException("no word service endpoint available")
    }

    private fun postPreferredWordJson(
        path: String,
        body: JSONObject,
        readTimeoutMs: Int,
    ): JSONObject {
        val baseUrls = WordServiceConfig.preferredApiBaseUrls()
        var lastError: IOException? = null
        for ((index, baseUrl) in baseUrls.withIndex()) {
            try {
                return postJson(
                    baseUrl + path,
                    body,
                    readTimeoutMs = readTimeoutMs,
                    connectTimeoutMs = if (index == baseUrls.lastIndex) {
                        DEFAULT_CONNECT_TIMEOUT_MS
                    } else {
                        LOCAL_CONNECT_TIMEOUT_MS
                    },
                )
            } catch (e: ConnectionOpenException) {
                lastError = e
                if (index == baseUrls.lastIndex) throw e
                Log.w(TAG, "local word service unavailable url=${e.url}; falling back to public service")
            }
        }
        throw lastError ?: IOException("no word service endpoint available")
    }

    private fun getJson(
        urlStr: String,
        readTimeoutMs: Int = DEFAULT_READ_TIMEOUT_MS,
        connectTimeoutMs: Int = DEFAULT_CONNECT_TIMEOUT_MS,
    ): JSONObject {
        val t0 = System.currentTimeMillis()
        val url = URL(urlStr)
        val conn = url.openConnection() as HttpURLConnection
        try {
            conn.requestMethod = "GET"
            conn.doInput = true
            conn.doOutput = false
            conn.connectTimeout = connectTimeoutMs
            conn.readTimeout = readTimeoutMs
            conn.setRequestProperty("Accept", "application/json")
            conn.setRequestProperty("Connection", "close")
            try {
                conn.connect()
            } catch (e: IOException) {
                throw ConnectionOpenException(urlStr, e)
            }
            val code = conn.responseCode
            if (code !in 200..299) {
                val errText = runCatching {
                    conn.errorStream?.use { it.reader(Charsets.UTF_8).readText() }
                }.getOrNull().orEmpty()
                Log.w(
                    TAG,
                    "getJson non-2xx url=$urlStr code=$code msg=${conn.responseMessage} elapsed=${System.currentTimeMillis() - t0}ms body=${errText.take(500)}",
                )
                throw IOException("HTTP $code ${conn.responseMessage ?: ""}")
            }
            val text = conn.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            Log.d(
                TAG,
                "getJson ok url=$urlStr code=$code elapsed=${System.currentTimeMillis() - t0}ms resp_len=${text.length}",
            )
            return JSONObject(text)
        } catch (t: Throwable) {
            Log.w(
                TAG,
                "getJson failed url=$urlStr readTimeoutMs=$readTimeoutMs elapsed=${System.currentTimeMillis() - t0}ms err_type=${t.javaClass.name} err_msg=${t.message}",
                t,
            )
            throw t
        } finally {
            conn.disconnect()
        }
    }

    private fun postJson(
        urlStr: String,
        body: JSONObject,
        readTimeoutMs: Int = DEFAULT_READ_TIMEOUT_MS,
        connectTimeoutMs: Int = DEFAULT_CONNECT_TIMEOUT_MS,
    ): JSONObject {
        val t0 = System.currentTimeMillis()
        val url = URL(urlStr)
        val conn = url.openConnection() as HttpURLConnection
        try {
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.doInput = true
            conn.connectTimeout = connectTimeoutMs
            conn.readTimeout = readTimeoutMs
            conn.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            conn.setRequestProperty("Accept", "application/json")
            // 显式关闭 HttpURLConnection 的连接复用池，避免长耗时 LLM 请求命中
            // 已被服务端半关的空闲 socket 导致读响应体时抛 EOF/StreamResetException
            conn.setRequestProperty("Connection", "close")
            try {
                conn.connect()
            } catch (e: IOException) {
                throw ConnectionOpenException(urlStr, e)
            }
            conn.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            val code = conn.responseCode
            if (code !in 200..299) {
                val errText = runCatching {
                    conn.errorStream?.use { it.reader(Charsets.UTF_8).readText() }
                }.getOrNull().orEmpty()
                Log.w(
                    TAG,
                    "postJson non-2xx url=$urlStr code=$code msg=${conn.responseMessage} elapsed=${System.currentTimeMillis() - t0}ms body=${errText.take(500)}",
                )
                throw IOException("HTTP $code ${conn.responseMessage ?: ""}")
            }
            val text = conn.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            Log.d(
                TAG,
                "postJson ok url=$urlStr code=$code elapsed=${System.currentTimeMillis() - t0}ms resp_len=${text.length}",
            )
            return JSONObject(text)
        } catch (t: Throwable) {
            Log.w(
                TAG,
                "postJson failed url=$urlStr readTimeoutMs=$readTimeoutMs elapsed=${System.currentTimeMillis() - t0}ms err_type=${t.javaClass.name} err_msg=${t.message}",
                t,
            )
            throw t
        } finally {
            conn.disconnect()
        }
    }

    private fun putJson(
        urlStr: String,
        body: JSONObject,
        readTimeoutMs: Int = DEFAULT_READ_TIMEOUT_MS,
    ): JSONObject {
        val t0 = System.currentTimeMillis()
        val conn = URL(urlStr).openConnection() as HttpURLConnection
        try {
            conn.requestMethod = "PUT"
            conn.doOutput = true
            conn.doInput = true
            conn.connectTimeout = 8000
            conn.readTimeout = readTimeoutMs
            conn.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            conn.setRequestProperty("Accept", "application/json")
            conn.setRequestProperty("Connection", "close")
            conn.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            val code = conn.responseCode
            val text = if (code in 200..299) {
                conn.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                conn.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                Log.w(
                    TAG,
                    "putJson non-2xx url=$urlStr code=$code msg=${conn.responseMessage} elapsed=${System.currentTimeMillis() - t0}ms body=${text.take(500)}",
                )
                throw IOException("HTTP $code ${runCatching { JSONObject(text).optString("msg") }.getOrDefault("")}")
            }
            Log.d(TAG, "putJson ok url=$urlStr code=$code elapsed=${System.currentTimeMillis() - t0}ms")
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        } catch (t: Throwable) {
            Log.w(
                TAG,
                "putJson failed url=$urlStr elapsed=${System.currentTimeMillis() - t0}ms err_type=${t.javaClass.name} err_msg=${t.message}",
                t,
            )
            throw t
        } finally {
            conn.disconnect()
        }
    }

    private fun parseConfusableWordsData(data: JSONObject): ConfusableWordsData {
        val members = mutableListOf<ConfusableWord>()
        data.optJSONArray("members")?.let { array ->
            for (i in 0 until array.length()) {
                val item = array.optJSONObject(i) ?: continue
                members += ConfusableWord(
                    itemId = item.optLong("itemId", 0L),
                    word = item.optString("word", "").trim(),
                    topMeaningText = item.optString("topMeaningText", "").trim(),
                )
            }
        }
        return ConfusableWordsData(
            groupId = data.optLong("id", 0L),
            members = sanitizeConfusableWords(members),
        )
    }

    private const val TAG = "WordApi"
    private const val LOCAL_CONNECT_TIMEOUT_MS = 1_200
    private const val DEFAULT_CONNECT_TIMEOUT_MS = 8_000
    private const val DEFAULT_READ_TIMEOUT_MS = 15_000
    private const val LLM_READ_TIMEOUT_MS = 60_000
    private const val WORD_INITIALIZATION_READ_TIMEOUT_MS = 45_000
    private const val CONTEXT_TASK_READ_TIMEOUT_MS = 120_000
}

private class ConnectionOpenException(
    val url: String,
    cause: IOException,
) : IOException("connect failed: $url", cause)
