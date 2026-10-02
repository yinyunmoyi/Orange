package com.example.orange.data.offline

import android.content.Context
import com.google.ai.edge.litertlm.Backend
import com.google.ai.edge.litertlm.Contents
import com.google.ai.edge.litertlm.ConversationConfig
import com.google.ai.edge.litertlm.Engine
import com.google.ai.edge.litertlm.EngineConfig
import com.google.ai.edge.litertlm.MessageCallback
import com.google.ai.edge.litertlm.ThinkingConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.withContext
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.coroutines.coroutineContext
import org.json.JSONObject

internal interface LiteRtEngineHandle {
    val isInitialized: Boolean
    fun initialize()
    fun generate(systemPrompt: String, userPrompt: String, maxTokens: Int): Flow<String>
    fun close()
}

internal fun interface LiteRtEngineHandleFactory {
    fun create(modelPath: String, backend: OfflineBackend): LiteRtEngineHandle
}

class LiteRtOfflineInferenceEngine internal constructor(
    private val engineFactory: LiteRtEngineHandleFactory,
) : OfflineInferenceEngine {
    private var diagnosticTraceId: String? = null

    constructor(context: Context, traceId: String? = null) : this(
        engineFactory = realEngineFactory(context.applicationContext),
    ) {
        diagnosticTraceId = traceId
    }

    private var engine: LiteRtEngineHandle? = null

    override suspend fun initialize(modelPath: String): Result<OfflineBackend> =
        withContext(Dispatchers.IO) {
            close()
            val gpuStartNanos = System.nanoTime()
            OfflineDiagnostics.event("backend_initialization_started", diagnosticTraceId, backendData(
                OfflineBackend.GPU,
            ))
            val gpuFailure = runCatching {
                createInitializedEngine(modelPath, OfflineBackend.GPU)
            }
            gpuFailure.getOrNull()?.let {
                engine = it
                OfflineDiagnostics.event(
                    "backend_initialization_completed",
                    diagnosticTraceId,
                    backendData(
                        OfflineBackend.GPU,
                        (System.nanoTime() - gpuStartNanos) / 1_000_000L,
                    ),
                )
                return@withContext Result.success(OfflineBackend.GPU)
            }
            OfflineDiagnostics.failure(
                "backend_initialization_failed",
                diagnosticTraceId,
                checkNotNull(gpuFailure.exceptionOrNull()),
                backendData(
                    OfflineBackend.GPU,
                    (System.nanoTime() - gpuStartNanos) / 1_000_000L,
                ),
            )

            coroutineContext.ensureActive()
            val cpuStartNanos = System.nanoTime()
            OfflineDiagnostics.event("backend_initialization_started", diagnosticTraceId, backendData(
                OfflineBackend.CPU,
            ))
            val cpuFailure = runCatching {
                createInitializedEngine(modelPath, OfflineBackend.CPU)
            }
            cpuFailure.fold(
                onSuccess = {
                    engine = it
                    OfflineDiagnostics.event(
                        "backend_initialization_completed",
                        diagnosticTraceId,
                        backendData(
                            OfflineBackend.CPU,
                            (System.nanoTime() - cpuStartNanos) / 1_000_000L,
                        ),
                    )
                    Result.success(OfflineBackend.CPU)
                },
                onFailure = { cpuError ->
                    OfflineDiagnostics.failure(
                        "backend_initialization_failed",
                        diagnosticTraceId,
                        cpuError,
                        backendData(
                            OfflineBackend.CPU,
                            (System.nanoTime() - cpuStartNanos) / 1_000_000L,
                        ),
                    )
                    Result.failure(
                        IllegalStateException(
                            "GPU 初始化失败：${gpuFailure.exceptionOrNull().messageOrType()}；" +
                                "CPU 初始化失败：${cpuError.messageOrType()}",
                            cpuError,
                        ),
                    )
                },
            )
        }

    override fun generate(
        systemPrompt: String,
        userPrompt: String,
        maxTokens: Int,
    ): Flow<String> = flow {
        require(maxTokens > 0) { "maxTokens must be positive" }
        val currentEngine = checkNotNull(engine) { "Offline engine is not initialized" }
        currentEngine.generate(systemPrompt, userPrompt, maxTokens).collect { text ->
            if (text.isNotEmpty()) emit(text)
        }
    }.flowOn(Dispatchers.IO)

    override suspend fun close() = withContext(Dispatchers.IO) {
        val current = engine
        engine = null
        if (current?.isInitialized == true) {
            val closeStartNanos = System.nanoTime()
            runCatching { current.close() }
                .onSuccess {
                    OfflineDiagnostics.event(
                        "backend_closed",
                        diagnosticTraceId,
                        JSONObject().put(
                            "durationMillis",
                            (System.nanoTime() - closeStartNanos) / 1_000_000L,
                        ),
                    )
                }
                .onFailure {
                    OfflineDiagnostics.failure(
                        "backend_close_failed",
                        diagnosticTraceId,
                        it,
                    )
                }
        }
    }

    private fun createInitializedEngine(
        modelPath: String,
        backend: OfflineBackend,
    ): LiteRtEngineHandle {
        val candidate = engineFactory.create(modelPath, backend)
        return try {
            candidate.initialize()
            candidate
        } catch (error: Throwable) {
            if (candidate.isInitialized) runCatching { candidate.close() }
            throw error
        }
    }
}

private fun backendData(backend: OfflineBackend, durationMillis: Long? = null): JSONObject =
    JSONObject()
        .put("backend", backend.name)
        .also { data ->
            if (durationMillis != null) data.put("durationMillis", durationMillis)
        }

private fun realEngineFactory(context: Context): LiteRtEngineHandleFactory {
    val cacheDirectory = context.cacheDir.resolve("litertlm").apply { mkdirs() }
    return LiteRtEngineHandleFactory { modelPath, backend ->
        RealLiteRtEngineHandle(
            Engine(
                EngineConfig(
                    modelPath = modelPath,
                    backend = when (backend) {
                        OfflineBackend.GPU -> Backend.GPU()
                        OfflineBackend.CPU -> Backend.CPU()
                    },
                    cacheDir = cacheDirectory.absolutePath,
                ),
            ),
        )
    }
}

private class RealLiteRtEngineHandle(
    private val engine: Engine,
) : LiteRtEngineHandle {
    override val isInitialized: Boolean
        get() = engine.isInitialized()

    override fun initialize() = engine.initialize()

    override fun generate(
        systemPrompt: String,
        userPrompt: String,
        maxTokens: Int,
    ): Flow<String> = callbackFlow {
        val conversation = engine.createConversation(
            ConversationConfig(
                systemInstruction = Contents.of(systemPrompt),
                maxOutputToken = maxTokens,
                thinkingConfig = ThinkingConfig(enableThinking = false, thinkingTokenBudget = 0),
            ),
        )
        val terminated = AtomicBoolean(false)
        val released = AtomicBoolean(false)
        fun release(cancel: Boolean) {
            if (!released.compareAndSet(false, true)) return
            if (cancel) runCatching { conversation.cancelProcess() }
            runCatching { conversation.close() }
        }
        try {
            conversation.sendMessageAsync(
                text = userPrompt,
                callback = object : MessageCallback {
                    override fun onMessage(message: com.google.ai.edge.litertlm.Message) {
                        trySend(message.toString())
                    }

                    override fun onDone() {
                        terminated.set(true)
                        channel.close(null)
                    }

                    override fun onError(throwable: Throwable) {
                        terminated.set(true)
                        channel.close(throwable)
                    }
                },
                maxOutputToken = maxTokens,
                thinkingConfig = ThinkingConfig(enableThinking = false, thinkingTokenBudget = 0),
            )
        } catch (error: Throwable) {
            terminated.set(true)
            release(cancel = true)
            throw error
        }
        awaitClose { release(cancel = !terminated.get()) }
    }

    override fun close() = engine.close()
}

private fun Throwable?.messageOrType(): String =
    this?.message?.takeIf(String::isNotBlank) ?: this?.javaClass?.simpleName ?: "未知错误"
