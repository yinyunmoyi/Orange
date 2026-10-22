package com.example.orange.data.learning

import android.content.Context
import android.os.SystemClock
import com.example.orange.data.logging.AppLog as Log
import com.example.orange.data.contextsync.ContextSyncPreferences
import com.example.orange.data.network.NetworkAccessPolicy
import com.example.orange.data.reverselan.REVERSE_LAN_INTERACTIVE_LANE
import com.example.orange.data.reverselan.ReverseLanManager
import com.example.orange.data.reverselan.ReverseLanRequest
import com.example.orange.data.reverselan.ReverseLanResult
import com.example.orange.data.word.ContextSentence
import com.example.orange.data.word.parseContextVideos
import com.example.orange.data.word.WordServiceConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.ConnectException
import java.io.IOException
import java.net.NoRouteToHostException
import java.net.HttpURLConnection
import java.net.SocketTimeoutException
import java.net.URL

object LearningApi {
    private val mutationCoordinator = LearningMutationCoordinator()

    @Volatile
    private var applicationContext: Context? = null

    fun init(context: Context) {
        applicationContext = context.applicationContext
    }

    suspend fun getSettings(): Result<LearningSettings> = withContext(Dispatchers.IO) {
        runCatching {
            parseLearningSettingsResponse(request("GET", "$baseUrl/settings"))
        }
    }

    suspend fun updateSettings(
        dailyNewLimit: Int,
        dailyReviewLimit: Int,
    ): Result<LearningSettings> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("dailyNewLimit", dailyNewLimit)
                .put("dailyReviewLimit", dailyReviewLimit)
            parseLearningSettingsResponse(request("PUT", "$baseUrl/settings", body))
        }
    }

    suspend fun createSession(): Result<LearningPlan> = withContext(Dispatchers.IO) {
        runCatching {
            parseLearningPlan(request("POST", "$baseUrl/sessions", JSONObject()))
        }
    }

    suspend fun extendSession(sessionId: Long): Result<LearningPlan> = withContext(Dispatchers.IO) {
        runCatching {
            parseLearningPlan(request("POST", "$baseUrl/sessions/$sessionId/extend", JSONObject()))
        }
    }

    suspend fun currentCard(sessionId: Long): Result<LearningCurrent> = withContext(Dispatchers.IO) {
        runCatching {
            parseCurrent(
                requireLearningSuccess(
                    dispatchLearning(
                        method = "GET",
                        path = "/api/v1/learning/sessions/$sessionId/current",
                        body = null,
                        traceId = null,
                    ),
                ),
            )
        }
    }

    suspend fun answer(
        sessionId: Long,
        queueItemId: Long,
        turnNo: Int,
        answer: String,
        traceId: String? = null,
    ): Result<LearningCurrent> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("queueItemId", queueItemId)
                .put("turnNo", turnNo)
                .put("answer", answer)
            val path = "/api/v1/learning/sessions/$sessionId/answers"
            parseCurrent(
                mutationCoordinator.execute(
                    sessionId = sessionId,
                    expectedQueueItemId = queueItemId,
                    expectedTurnNo = turnNo,
                    dispatch = { dispatchLearning("POST", path, body, traceId) },
                    readCurrent = {
                        requireLearningSuccess(
                            dispatchLearning(
                                "GET",
                                "/api/v1/learning/sessions/$sessionId/current",
                                null,
                                traceId,
                            ),
                        )
                    },
                    retryPublic = {
                        requestOnce(
                            "POST",
                            WordServiceConfig.origin + path,
                            body,
                            readTimeoutMillis = 20_000,
                            traceId = traceId,
                        )
                    },
                ),
            )
        }
    }

    suspend fun masterCurrentItem(
        sessionId: Long,
        queueItemId: Long,
        turnNo: Int,
        traceId: String? = null,
    ): Result<LearningCurrent> = withContext(Dispatchers.IO) {
        runCatching {
            val body = JSONObject()
                .put("queueItemId", queueItemId)
                .put("turnNo", turnNo)
            val path = "/api/v1/learning/sessions/$sessionId/mastery"
            parseCurrent(
                mutationCoordinator.execute(
                    sessionId = sessionId,
                    expectedQueueItemId = queueItemId,
                    expectedTurnNo = turnNo,
                    dispatch = { dispatchLearning("POST", path, body, traceId) },
                    readCurrent = {
                        requireLearningSuccess(
                            dispatchLearning(
                                "GET",
                                "/api/v1/learning/sessions/$sessionId/current",
                                null,
                                traceId,
                            ),
                        )
                    },
                    retryPublic = {
                        requestOnce(
                            "POST",
                            WordServiceConfig.origin + path,
                            body,
                            readTimeoutMillis = 20_000,
                            traceId = traceId,
                        )
                    },
                ),
            )
        }
    }

    suspend fun regenerateAudio(itemType: Int, itemId: Long): Result<String> = withContext(Dispatchers.IO) {
        runCatching {
            val pathType = when (itemType) {
                LEARNING_ITEM_WORD -> "word"
                LEARNING_ITEM_PHRASE -> "phrase"
                else -> throw IllegalArgumentException("invalid learning item type")
            }
            require(itemId > 0L) { "invalid learning item id" }
            val json = request(
                method = "POST",
                urlString = "${WordServiceConfig.origin}/api/v1/favorites/$pathType/$itemId/audio/regenerate",
                body = JSONObject(),
                readTimeoutMillis = 75_000,
            )
            json.requireData().optString("audioUrl").trim().ifEmpty {
                throw IOException("response missing audioUrl")
            }
        }
    }

    private suspend fun dispatchLearning(
        method: String,
        path: String,
        body: JSONObject?,
        traceId: String?,
    ): LearningMutationDispatch {
        val localOrigin = applicationContext
            ?.let(::ContextSyncPreferences)
            ?.localOrigin
            ?.trimEnd('/')
        if (!localOrigin.isNullOrEmpty()) {
            when (
                val attempt = requestOnceOutcome(
                    method = method,
                    urlString = localOrigin + path,
                    body = body,
                    connectTimeoutMillis = 1_000,
                    traceId = traceId,
                )
            ) {
                is LearningHttpAttempt.Response -> {
                    reportEncodingAnomaly(path, "forward", attempt.body, traceId)
                    reportTransport(method, path, "forward", "response", traceId)
                    return LearningMutationDispatch.Success(attempt.requireJson())
                }
                is LearningHttpAttempt.NotDispatched ->
                    reportTransport(method, path, "forward", "not_dispatched", traceId)
                is LearningHttpAttempt.DeliveryUnknown -> {
                    reportTransport(method, path, "forward", "delivery_unknown", traceId)
                    if (method != "GET") {
                        return LearningMutationDispatch.DeliveryUnknown(
                            attempt.error,
                            canRetryAfterReconcile = false,
                        )
                    }
                }
            }
        }

        val reverseRequest = ReverseLanRequest(
            lane = REVERSE_LAN_INTERACTIVE_LANE,
            method = method,
            path = path,
            headers = buildMap {
                put("Accept", "application/json")
                if (body != null) put("Content-Type", "application/json; charset=utf-8")
                if (traceId != null) put(DEBUG_TRACE_HEADER, traceId)
            },
            body = body?.toString().orEmpty(),
        )
        when (val result = ReverseLanManager.execute(reverseRequest)) {
            is ReverseLanResult.Response -> {
                reportEncodingAnomaly(path, "reverse", result.value.body, traceId)
                reportTransport(method, path, "reverse", "response", traceId)
                return LearningMutationDispatch.Success(
                    LearningHttpAttempt.Response(
                        result.value.status,
                        result.value.body,
                    ).requireJson(),
                )
            }
            ReverseLanResult.DeliveryUnknown -> {
                reportTransport(method, path, "reverse", "delivery_unknown", traceId)
                if (method != "GET") {
                    return LearningMutationDispatch.DeliveryUnknown(
                        IOException("reverse LAN delivery outcome is unknown"),
                        canRetryAfterReconcile = true,
                    )
                }
            }
            ReverseLanResult.NotDispatched ->
                reportTransport(method, path, "reverse", "not_dispatched", traceId)
            ReverseLanResult.Unavailable ->
                reportTransport(method, path, "reverse", "unavailable", traceId)
        }

        return when (
            val attempt = requestOnceOutcome(
                method = method,
                urlString = WordServiceConfig.origin + path,
                body = body,
                connectTimeoutMillis = 8_000,
                traceId = traceId,
            )
        ) {
            is LearningHttpAttempt.Response -> {
                reportEncodingAnomaly(path, "public", attempt.body, traceId)
                reportTransport(method, path, "public", "response", traceId)
                LearningMutationDispatch.Success(attempt.requireJson())
            }
            is LearningHttpAttempt.NotDispatched -> {
                reportTransport(method, path, "public", "not_dispatched", traceId)
                throw attempt.error
            }
            is LearningHttpAttempt.DeliveryUnknown -> {
                reportTransport(method, path, "public", "delivery_unknown", traceId)
                if (method == "GET") throw attempt.error
                LearningMutationDispatch.DeliveryUnknown(
                    attempt.error,
                    canRetryAfterReconcile = false,
                )
            }
        }
    }

    private fun reportTransport(
        method: String,
        path: String,
        transport: String,
        outcome: String,
        traceId: String?,
    ) {
        reportEvent(
            location = "LearningApi:dispatchLearning",
            message = "learning transport result",
            data = JSONObject()
                .put("method", method)
                .put("path", path)
                .put("transport", transport)
                .put("outcome", outcome),
            traceId = traceId,
            immediateUpload = true,
        )
    }

    private fun reportEncodingAnomaly(
        path: String,
        transport: String,
        body: String,
        traceId: String?,
    ) {
        val replacementCount = body.count { it == '\uFFFD' }
        if (replacementCount == 0) return
        reportEvent(
            location = "LearningApi:dispatchLearning",
            message = "learning response contains replacement characters",
            data = JSONObject()
                .put("path", path)
                .put("transport", transport)
                .put("replacementCount", replacementCount),
            traceId = traceId,
            immediateUpload = true,
        )
    }

    private fun requireLearningSuccess(dispatch: LearningMutationDispatch): JSONObject =
        when (dispatch) {
            is LearningMutationDispatch.Success -> dispatch.json
            is LearningMutationDispatch.DeliveryUnknown ->
                throw IOException("learning request delivery outcome is unknown", dispatch.cause)
        }

    private fun LearningHttpAttempt.Response.requireJson(): JSONObject {
        if (status !in 200..299) {
            val message = runCatching { JSONObject(body).optString("msg") }.getOrDefault("")
            throw LearningHttpStatusException(status, "HTTP $status $message")
        }
        return JSONObject(body)
    }

    private fun parseLearningPlan(json: JSONObject): LearningPlan {
        val data = json.requireData()
        val counts = data.optJSONObject("statusCounts") ?: JSONObject()
        return LearningPlan(
            sessionId = data.optLong("sessionId"),
            sessionStatus = data.optString("sessionStatus"),
            todayTotal = data.optInt("todayTotal"),
            todayNew = data.optInt("todayNew"),
            todayReview = data.optInt("todayReview"),
            todayRetry = data.optInt("todayRetry"),
            totalItems = data.optInt("totalItems"),
            statusCounts = LearningStatusCounts(
                notStarted = counts.optInt("notStarted"),
                learning = counts.optInt("learning"),
                learned = counts.optInt("learned"),
                mastered = counts.optInt("mastered"),
            ),
            hasCurrentCard = data.optBoolean("hasCurrentCard"),
            canExtend = data.optBoolean("canExtend"),
        )
    }

    internal fun parseCurrent(json: JSONObject): LearningCurrent {
        val data = json.requireData()
        val remaining = data.optInt("remaining")
        val progress = data.optJSONObject("progress")
        return LearningCurrent(
            sessionId = data.optLong("sessionId"),
            turnNo = data.optInt("turnNo"),
            queueItemId = data.optLong("queueItemId"),
            queueType = data.optString("queueType"),
            remaining = remaining,
            completed = data.optBoolean("completed"),
            progress = LearningSessionProgress(
                total = progress?.optInt("total") ?: remaining,
                notStarted = progress?.optInt("notStarted") ?: remaining,
                inProgress = progress?.optInt("inProgress") ?: 0,
                completed = progress?.optInt("completed") ?: 0,
            ),
            card = data.optJSONObject("card")?.let(::parseCard),
        )
    }

    private fun parseCard(data: JSONObject): LearningCard {
        fun meanings(key: String): List<LearningMeaning> {
            val result = mutableListOf<LearningMeaning>()
            val array = data.optJSONArray(key) ?: return result
            for (i in 0 until array.length()) {
                val item = array.optJSONObject(i) ?: continue
                result += LearningMeaning(item.optString("partOfSpeech"), item.optString("text"))
            }
            return result
        }
        val contexts = mutableListOf<ContextSentence>()
        data.optJSONArray("contexts")?.let { array ->
            for (i in 0 until array.length()) {
                val item = array.optJSONObject(i) ?: continue
                contexts += ContextSentence(
                    id = item.optLong("id"),
                    itemType = item.optString("itemType", "word"),
                    itemId = item.optLong("itemId"),
                    sentence = item.optString("sentence"),
                    translation = item.optString("translation"),
                    highlight = item.optString("highlight"),
                    highlightStart = item.optInt("highlightStart", -1),
                    highlightEnd = item.optInt("highlightEnd", -1),
                    audioUrl = item.optString("audioUrl"),
                    contextType = item.optString("contextType", "text").ifBlank { "text" },
                    videos = parseContextVideos(item),
                )
            }
        }
        return LearningCard(
            itemType = data.optInt("itemType"),
            itemId = data.optLong("itemId"),
            word = data.optString("word"),
            level = data.optInt("level", 0),
            phonetic = data.optString("phonetic"),
            audioUrl = data.optString("audioUrl"),
            chinese = meanings("chinese"),
            contexts = contexts,
            english = meanings("english"),
            note = data.optString("note", ""),
            pos = data.optString("pos", "").trim(),
            collins = data.optInt("collins", 0),
            oxford = data.optInt("oxford", 0),
            tags = parseTagsArray(data),
        )
    }

    private fun parseTagsArray(data: JSONObject): List<String> {
        val array = data.optJSONArray("tags") ?: return emptyList()
        val result = mutableListOf<String>()
        for (i in 0 until array.length()) {
            val text = array.optString(i, "").trim()
            if (text.isNotEmpty()) result += text
        }
        return result
    }

    private fun JSONObject.requireData(): JSONObject =
        optJSONObject("data") ?: throw IOException(optString("msg", "response missing data"))

    private fun request(
        method: String,
        urlString: String,
        body: JSONObject? = null,
        readTimeoutMillis: Int = 20_000,
        traceId: String? = null,
    ): JSONObject {
        val publicOrigin = WordServiceConfig.origin
        val started = SystemClock.elapsedRealtime()
        reportEvent(
            location = "LearningApi:request",
            message = "public request started",
            data = JSONObject().put("method", method).put("origin", publicOrigin),
            traceId = traceId,
        )
        return try {
            requestOnce(method, urlString, body, readTimeoutMillis, traceId = traceId).also {
                reportEvent(
                    location = "LearningApi:request",
                    message = "public request completed",
                    data = JSONObject()
                        .put("method", method)
                        .put("origin", publicOrigin)
                        .put("durationMs", SystemClock.elapsedRealtime() - started),
                    traceId = traceId,
                )
            }
        } catch (error: Throwable) {
            reportEvent(
                location = "LearningApi:request",
                message = "public request failed",
                data = JSONObject()
                    .put("method", method)
                    .put("origin", publicOrigin)
                    .put("durationMs", SystemClock.elapsedRealtime() - started)
                    .put("errorType", error.javaClass.simpleName),
                traceId = traceId,
            )
            throw error
        }
    }

    private fun requestOnce(
        method: String,
        urlString: String,
        body: JSONObject?,
        readTimeoutMillis: Int,
        connectTimeoutMillis: Int = 8_000,
        traceId: String? = null,
    ): JSONObject {
        val started = System.currentTimeMillis()
        val connection = NetworkAccessPolicy.openConnection(URL(urlString))
        try {
            connection.requestMethod = method
            connection.connectTimeout = connectTimeoutMillis
            connection.readTimeout = readTimeoutMillis
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            traceId?.let { connection.setRequestProperty(DEBUG_TRACE_HEADER, it) }
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
                connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            }
            val code = connection.responseCode
            val text = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                Log.w(TAG, "$method failed url=$urlString code=$code body=${text.take(500)}")
                throw LearningHttpStatusException(
                    code,
                    "HTTP $code ${runCatching { JSONObject(text).optString("msg") }.getOrDefault("")}",
                )
            }
            return JSONObject(text)
        } catch (error: Throwable) {
            Log.e(TAG, "$method error url=$urlString elapsed=${System.currentTimeMillis() - started}ms", error)
            throw error
        } finally {
            NetworkAccessPolicy.release(connection)
        }
    }

    private fun requestOnceOutcome(
        method: String,
        urlString: String,
        body: JSONObject?,
        connectTimeoutMillis: Int,
        readTimeoutMillis: Int = 20_000,
        traceId: String? = null,
    ): LearningHttpAttempt {
        val connection = try {
            NetworkAccessPolicy.openConnection(URL(urlString))
        } catch (error: Throwable) {
            return LearningHttpAttempt.NotDispatched(error)
        }
        val bodyBytes = body?.toString()?.toByteArray(Charsets.UTF_8)
        try {
            connection.requestMethod = method
            connection.connectTimeout = connectTimeoutMillis
            connection.readTimeout = readTimeoutMillis
            connection.instanceFollowRedirects = false
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            traceId?.let { connection.setRequestProperty(DEBUG_TRACE_HEADER, it) }
            if (bodyBytes != null) {
                connection.doOutput = true
                connection.setFixedLengthStreamingMode(bodyBytes.size)
                connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            }
            try {
                connection.connect()
            } catch (error: Throwable) {
                return if (
                    error is ConnectException ||
                    error is NoRouteToHostException ||
                    error is SocketTimeoutException
                ) {
                    LearningHttpAttempt.NotDispatched(error)
                } else {
                    LearningHttpAttempt.DeliveryUnknown(error)
                }
            }
            return try {
                if (bodyBytes != null) {
                    connection.outputStream.use { it.write(bodyBytes) }
                }
                val status = connection.responseCode
                val text = if (status in 200..299) {
                    connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
                } else {
                    connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
                }
                LearningHttpAttempt.Response(status, text)
            } catch (error: Throwable) {
                LearningHttpAttempt.DeliveryUnknown(error)
            }
        } finally {
            NetworkAccessPolicy.release(connection)
        }
    }

    internal fun reportEvent(
        location: String,
        message: String,
        data: JSONObject = JSONObject(),
        traceId: String? = null,
        immediateUpload: Boolean = false,
    ) {
        Log.event(
            tag = "LearningTrace",
            message = message,
            data = JSONObject(data.toString()).put("location", location),
            traceId = traceId,
            immediateUpload = immediateUpload,
        )
    }

    private val baseUrl: String get() = "${WordServiceConfig.origin}/api/v1/learning"
    private const val TAG = "LearningApi"
    private const val DEBUG_TRACE_HEADER = "X-Orange-Debug-Trace-Id"
}

private sealed interface LearningHttpAttempt {
    data class Response(val status: Int, val body: String) : LearningHttpAttempt
    data class NotDispatched(val error: Throwable) : LearningHttpAttempt
    data class DeliveryUnknown(val error: Throwable) : LearningHttpAttempt
}

internal fun parseLearningSettingsResponse(json: JSONObject): LearningSettings {
    val data = json.optJSONObject("data")
        ?: throw IOException(json.optString("msg", "response missing data"))
    val dailyNewLimit = data.optInt("dailyNewLimit", -1)
    val dailyReviewLimit = data.optInt("dailyReviewLimit", -1)
    val remainingNewCount = data.optInt("remainingNewCount", -1)
    val studyDate = data.optString("studyDate").trim()
    if (
        dailyNewLimit <= 0 ||
        dailyReviewLimit <= 0 ||
        remainingNewCount < 0 ||
        !studyDate.matches(Regex("""\d{4}-\d{2}-\d{2}"""))
    ) {
        throw IOException("response contains invalid learning settings")
    }
    return LearningSettings(
        dailyNewLimit = dailyNewLimit,
        dailyReviewLimit = dailyReviewLimit,
        remainingNewCount = remainingNewCount,
        studyDate = studyDate,
    )
}
