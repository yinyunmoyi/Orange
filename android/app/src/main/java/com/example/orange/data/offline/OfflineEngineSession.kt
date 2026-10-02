package com.example.orange.data.offline

import android.content.Context
import com.example.orange.data.mode.AppMode
import com.example.orange.data.mode.AppModeRepository
import java.io.IOException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeout
import org.json.JSONObject

object OfflineEngineSession {
    private val mutex = Mutex()
    private val closeScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private lateinit var engineFactory: OfflineInferenceEngineFactory
    private var engine: OfflineInferenceEngine? = null
    private var loadedPath: String? = null

    fun init(context: Context) {
        if (::engineFactory.isInitialized) return
        synchronized(this) {
            if (::engineFactory.isInitialized) return
            val appContext = context.applicationContext
            engineFactory = OfflineInferenceEngineFactory {
                LiteRtOfflineInferenceEngine(appContext)
            }
        }
    }

    suspend fun generate(
        systemPrompt: String,
        userPrompt: String,
        maxTokens: Int,
    ): String = mutex.withLock {
        val traceId = OfflineDiagnostics.newAttemptId()
        val startedAt = android.os.SystemClock.elapsedRealtime()
        checkInitialized()
        check(AppModeRepository.current() == AppMode.OFFLINE) {
            "Offline inference requires OFFLINE mode"
        }
        val result = OfflineCapabilityRepository.verifiedResult()
            ?: throw IOException("离线模型尚未验证")
        // #region debug-point D:engine-state
        OfflineDiagnostics.event(
            "debug_offline_generation_started",
            traceId,
            JSONObject()
                .put("runId", "post-fix")
                .put("hypothesisId", "D")
                .put("engineWarm", engine != null && loadedPath == result.fingerprint.path)
                .put("maxTokens", maxTokens)
                .put("systemPromptChars", systemPrompt.length)
                .put("userPromptChars", userPrompt.length),
        )
        // #endregion
        val currentEngine = ensureEngine(result.fingerprint.path)
        try {
            var firstTokenMillis = -1L
            var chunkCount = 0
            val response = withTimeout(GENERATION_TIMEOUT_MILLIS) {
                buildString {
                    currentEngine.generate(systemPrompt, userPrompt, maxTokens).collect { chunk ->
                        if (firstTokenMillis < 0L && chunk.isNotEmpty()) {
                            firstTokenMillis = android.os.SystemClock.elapsedRealtime() - startedAt
                        }
                        chunkCount++
                        append(chunk)
                        if (length > MAX_RESPONSE_CHARS) {
                            throw IOException("本地模型输出过长")
                        }
                    }
                }.trim().ifEmpty { throw IOException("本地模型返回了空内容") }
            }
            // #region debug-point B,C,D:generation-result
            OfflineDiagnostics.event(
                "debug_offline_generation_completed",
                traceId,
                JSONObject()
                    .put("runId", "post-fix")
                    .put("hypothesisId", "B,C,D")
                    .put("durationMillis", android.os.SystemClock.elapsedRealtime() - startedAt)
                    .put("firstTokenMillis", firstTokenMillis)
                    .put("chunkCount", chunkCount)
                    .put("responseChars", response.length)
                    .put("startsWithBrace", response.startsWith("{"))
                    .put("endsWithBrace", response.endsWith("}")),
            )
            // #endregion
            response
        } catch (error: Throwable) {
            // #region debug-point B,C:generation-failure
            OfflineDiagnostics.failure(
                "debug_offline_generation_failed",
                traceId,
                error,
                JSONObject()
                    .put("runId", "post-fix")
                    .put("hypothesisId", "B,C")
                    .put("durationMillis", android.os.SystemClock.elapsedRealtime() - startedAt)
                    .put("maxTokens", maxTokens),
            )
            // #endregion
            if (error is Error) closeLocked()
            throw error
        }
    }

    suspend fun warmUp(): Result<Unit> = mutex.withLock {
        runCatching {
            checkInitialized()
            val result = OfflineCapabilityRepository.verifiedResult()
                ?: throw IOException("离线模型尚未验证")
            val alreadyWarm = engine != null && loadedPath == result.fingerprint.path
            val startedAt = android.os.SystemClock.elapsedRealtime()
            ensureEngine(result.fingerprint.path)
            OfflineDiagnostics.event(
                "offline_engine_warmup_completed",
                null,
                JSONObject()
                    .put("alreadyWarm", alreadyWarm)
                    .put(
                        "durationMillis",
                        android.os.SystemClock.elapsedRealtime() - startedAt,
                    ),
            )
        }
    }

    suspend fun close() = mutex.withLock {
        closeLocked()
    }

    fun closeAsync() {
        if (!::engineFactory.isInitialized) return
        closeScope.launch { close() }
    }

    private suspend fun ensureEngine(modelPath: String): OfflineInferenceEngine {
        engine?.takeIf { loadedPath == modelPath }?.let { return it }
        closeLocked()
        val candidate = engineFactory.create()
        try {
            candidate.initialize(modelPath).getOrThrow()
            engine = candidate
            loadedPath = modelPath
            return candidate
        } catch (error: Throwable) {
            runCatching { candidate.close() }
            throw error
        }
    }

    private suspend fun closeLocked() {
        val current = engine
        engine = null
        loadedPath = null
        if (current != null) runCatching { current.close() }
    }

    private fun checkInitialized() {
        check(::engineFactory.isInitialized) { "OfflineEngineSession.init must be called first" }
    }

    private const val GENERATION_TIMEOUT_MILLIS = 180_000L
    private const val MAX_RESPONSE_CHARS = 65_536
}
