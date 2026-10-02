package com.example.orange.data.offline

import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.OpenableColumns
import java.io.File
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import org.json.JSONObject

enum class OfflineCapabilityStage {
    IDLE,
    IMPORTING_MODEL,
    CHECKING_FILE,
    VERIFYING_MODEL,
    CHECKING_DEVICE,
    INITIALIZING_ENGINE,
    TESTING_CONVERSATION,
    VERIFIED,
    ERROR,
}

data class OfflineCapabilityState(
    val stage: OfflineCapabilityStage = OfflineCapabilityStage.IDLE,
    val result: OfflineCapabilityResult? = null,
    val errorMessage: String? = null,
) {
    val inProgress: Boolean
        get() = stage in setOf(
            OfflineCapabilityStage.IMPORTING_MODEL,
            OfflineCapabilityStage.CHECKING_FILE,
            OfflineCapabilityStage.VERIFYING_MODEL,
            OfflineCapabilityStage.CHECKING_DEVICE,
            OfflineCapabilityStage.INITIALIZING_ENGINE,
            OfflineCapabilityStage.TESTING_CONVERSATION,
        )
}

class OfflineCapabilityVerifier(
    private val modelManager: OfflineModelManager,
    private val engineFactory: OfflineInferenceEngineFactory,
    private val appVersionCode: Long,
    private val elapsedRealtime: () -> Long,
    private val timeoutMillis: Long = OfflineModelSpec.VALIDATION_TIMEOUT_MILLIS,
) {
    suspend fun verify(
        cachedFingerprint: OfflineModelFingerprint?,
        onStage: (OfflineCapabilityStage) -> Unit,
        traceId: String? = null,
    ): OfflineCapabilityResult = withTimeout(timeoutMillis) {
        val totalStart = elapsedRealtime()
        val fingerprint = withContext(Dispatchers.IO) {
            modelManager.validate(
                cachedFingerprint = cachedFingerprint,
                onStage = { stage ->
                    val capabilityStage = when (stage) {
                        OfflineValidationStage.CHECKING_FILE ->
                            OfflineCapabilityStage.CHECKING_FILE
                        OfflineValidationStage.VERIFYING_MODEL ->
                            OfflineCapabilityStage.VERIFYING_MODEL
                        OfflineValidationStage.CHECKING_DEVICE ->
                            OfflineCapabilityStage.CHECKING_DEVICE
                    }
                    onStage(capabilityStage)
                    OfflineDiagnostics.event(
                        "validation_stage_started",
                        traceId,
                        JSONObject().put("stage", capabilityStage.name),
                    )
                },
                onMetrics = { metrics ->
                    OfflineDiagnostics.event(
                        "model_and_device_checked",
                        traceId,
                        JSONObject()
                            .put("hashCacheHit", metrics.hashCacheHit)
                            .put("hashMillis", metrics.hashMillis)
                            .put("supportedAbis", metrics.supportedAbis.joinToString(","))
                            .put("totalMemoryBytes", metrics.totalMemoryBytes)
                            .put("usableSpaceBytes", metrics.usableSpaceBytes),
                    )
                },
            )
        }

        val engine = engineFactory.create()
        try {
            onStage(OfflineCapabilityStage.INITIALIZING_ENGINE)
            OfflineDiagnostics.event("engine_initialization_started", traceId)
            val initializationStart = elapsedRealtime()
            val backend = engine.initialize(fingerprint.path).getOrThrow()
            val initializationMillis = elapsedRealtime() - initializationStart
            OfflineDiagnostics.event(
                "engine_initialization_completed",
                traceId,
                JSONObject()
                    .put("backend", backend.name)
                    .put("durationMillis", initializationMillis),
            )

            onStage(OfflineCapabilityStage.TESTING_CONVERSATION)
            OfflineDiagnostics.event(
                "validation_generation_started",
                traceId,
                JSONObject().put("maxTokens", OfflineModelSpec.MAX_VALIDATION_TOKENS),
            )
            var firstTokenMillis = -1L
            var chunkCount = 0
            var replyChars = 0
            val generationStart = elapsedRealtime()
            val reply = StringBuilder()
            engine.generate(
                systemPrompt = OfflineModelSpec.SYSTEM_PROMPT,
                userPrompt = OfflineModelSpec.USER_PROMPT,
                maxTokens = OfflineModelSpec.MAX_VALIDATION_TOKENS,
            ).collect { chunk ->
                if (chunk.isNotEmpty()) {
                    chunkCount++
                    replyChars += chunk.length
                    if (firstTokenMillis < 0L) {
                        firstTokenMillis = elapsedRealtime() - generationStart
                        OfflineDiagnostics.event(
                            "validation_first_token",
                            traceId,
                            JSONObject().put("firstTokenMillis", firstTokenMillis),
                        )
                    }
                    reply.append(chunk)
                }
            }
            val normalizedReply = reply.toString().trim()
            check(normalizedReply.isNotEmpty()) { "本地模型返回了空内容" }
            OfflineDiagnostics.event(
                "validation_generation_completed",
                traceId,
                JSONObject()
                    .put("durationMillis", elapsedRealtime() - generationStart)
                    .put("firstTokenMillis", firstTokenMillis)
                    .put("chunkCount", chunkCount)
                    .put("replyChars", replyChars),
            )

            OfflineCapabilityResult(
                fingerprint = fingerprint,
                appVersionCode = appVersionCode,
                backend = backend,
                reply = normalizedReply,
                initializationMillis = initializationMillis,
                firstTokenMillis = firstTokenMillis,
                totalMillis = elapsedRealtime() - totalStart,
            )
        } finally {
            OfflineDiagnostics.event("engine_release_started", traceId)
            engine.close()
            OfflineDiagnostics.event("engine_release_completed", traceId)
        }
    }
}

object OfflineCapabilityRepository {
    private val verificationMutex = Mutex()
    private val mutableState = MutableStateFlow(OfflineCapabilityState())
    private lateinit var preferences: OfflineCapabilityPreferences
    private lateinit var verifier: OfflineCapabilityVerifier
    private lateinit var modelManager: OfflineModelManager
    private lateinit var appContext: Context
    private var currentAppVersionCode: Long = -1L
    private var importedFingerprint: OfflineModelFingerprint? = null
    private var activeAttemptId: String? = null

    val state: StateFlow<OfflineCapabilityState> = mutableState.asStateFlow()
    val enabled: StateFlow<Boolean>
        get() = preferences.enabled

    fun verifiedResult(): OfflineCapabilityResult? {
        checkInitialized()
        return preferences.result.value
    }

    fun init(context: Context) {
        if (::preferences.isInitialized) return
        synchronized(this) {
            if (::preferences.isInitialized) return
            appContext = context.applicationContext
            modelManager = OfflineModelManager.create(appContext)
            modelManager.ensureModelDirectory()
            preferences = OfflineCapabilityPreferences(appContext)
            currentAppVersionCode = appVersionCode(appContext)
            val model = modelManager.modelFile()
            val invalidated = preferences.invalidateIfStale(
                modelFile = model?.takeIf(File::isFile)?.let {
                    FileMetadata(it.absolutePath, it.length(), it.lastModified())
                },
                appVersionCode = currentAppVersionCode,
            )
            verifier = OfflineCapabilityVerifier(
                modelManager = modelManager,
                engineFactory = OfflineInferenceEngineFactory {
                    LiteRtOfflineInferenceEngine(appContext, activeAttemptId)
                },
                appVersionCode = currentAppVersionCode,
                elapsedRealtime = android.os.SystemClock::elapsedRealtime,
            )
            preferences.result.value?.let {
                mutableState.value = OfflineCapabilityState(
                    stage = OfflineCapabilityStage.VERIFIED,
                    result = it,
                )
            }
            OfflineDiagnostics.event(
                "repository_initialized",
                traceId = null,
                data = JSONObject()
                    .put("appVersionCode", currentAppVersionCode)
                    .put("modelPresent", model?.isFile == true)
                    .put("cachedVerificationPresent", preferences.result.value != null)
                    .put("staleVerificationInvalidated", invalidated),
            )
        }
    }

    fun beginModelSelection() {
        checkInitialized()
        activeAttemptId = OfflineDiagnostics.newAttemptId()
        OfflineDiagnostics.event(
            "model_picker_opened",
            activeAttemptId,
            JSONObject().put("expectedFileSizeBytes", OfflineModelSpec.FILE_SIZE_BYTES),
        )
    }

    fun modelSelectionCancelled() {
        checkInitialized()
        OfflineDiagnostics.event("model_picker_cancelled", activeAttemptId)
        activeAttemptId = null
    }

    suspend fun importModel(uri: Uri): Result<OfflineModelFingerprint> = verificationMutex.withLock {
        checkInitialized()
        val traceId = activeAttemptId ?: OfflineDiagnostics.newAttemptId().also {
            activeAttemptId = it
        }
        val importStart = android.os.SystemClock.elapsedRealtime()
        preferences.disable()
        mutableState.value = OfflineCapabilityState(stage = OfflineCapabilityStage.IMPORTING_MODEL)
        return@withLock try {
            val metadata = modelSourceMetadata(appContext, uri)
            OfflineDiagnostics.event(
                "model_selected",
                traceId,
                JSONObject()
                    .put("hasDisplayName", metadata.displayName != null)
                    .put("fileNameMatches", metadata.displayName == OfflineModelSpec.FILE_NAME)
                    .put("declaredLengthBytes", metadata.length ?: JSONObject.NULL)
                    .put("mimeType", appContext.contentResolver.getType(uri) ?: JSONObject.NULL),
            )
            val fingerprint = withContext(Dispatchers.IO) {
                modelManager.importModel(
                    displayName = metadata.displayName,
                    declaredLength = metadata.length,
                    openInputStream = { appContext.contentResolver.openInputStream(uri) },
                    onStorageChecked = { usableBytes, requiredBytes ->
                        OfflineDiagnostics.event(
                            "model_import_storage_checked",
                            traceId,
                            JSONObject()
                                .put("usableSpaceBytes", usableBytes)
                                .put("requiredSpaceBytes", requiredBytes),
                        )
                    },
                    onProgress = { copiedBytes, percent ->
                        OfflineDiagnostics.event(
                            "model_import_progress",
                            traceId,
                            JSONObject()
                                .put("copiedBytes", copiedBytes)
                                .put("percent", percent),
                        )
                    },
                )
            }
            importedFingerprint = fingerprint
            preferences.invalidateIfStale(
                modelFile = FileMetadata(
                    fingerprint.path,
                    fingerprint.length,
                    fingerprint.lastModified,
                ),
                appVersionCode = currentAppVersionCode,
            )
            mutableState.value = OfflineCapabilityState()
            OfflineDiagnostics.event(
                "model_import_completed",
                traceId,
                JSONObject()
                    .put("bytes", fingerprint.length)
                    .put(
                        "durationMillis",
                        android.os.SystemClock.elapsedRealtime() - importStart,
                    ),
            )
            Result.success(fingerprint)
        } catch (cancelled: CancellationException) {
            mutableState.value = OfflineCapabilityState()
            OfflineDiagnostics.event(
                "model_import_cancelled",
                traceId,
                JSONObject().put(
                    "durationMillis",
                    android.os.SystemClock.elapsedRealtime() - importStart,
                ),
            )
            activeAttemptId = null
            throw cancelled
        } catch (error: Throwable) {
            mutableState.value = OfflineCapabilityState(
                stage = OfflineCapabilityStage.ERROR,
                errorMessage = error.message?.takeIf(String::isNotBlank)
                    ?: error.javaClass.simpleName,
            )
            OfflineDiagnostics.failure(
                "model_import_failed",
                traceId,
                error,
                JSONObject()
                    .put("stage", OfflineCapabilityStage.IMPORTING_MODEL.name)
                    .put(
                        "durationMillis",
                        android.os.SystemClock.elapsedRealtime() - importStart,
                    ),
            )
            activeAttemptId = null
            Result.failure(error)
        }
    }

    suspend fun verify(): Result<OfflineCapabilityResult> = verificationMutex.withLock {
        checkInitialized()
        val traceId = activeAttemptId ?: OfflineDiagnostics.newAttemptId().also {
            activeAttemptId = it
        }
        val verificationStart = android.os.SystemClock.elapsedRealtime()
        preferences.disable()
        return@withLock try {
            val result = verifier.verify(
                cachedFingerprint = importedFingerprint ?: preferences.cachedFingerprint(),
                onStage = { stage ->
                    mutableState.value = OfflineCapabilityState(stage = stage)
                },
                traceId = traceId,
            )
            preferences.save(result)
            mutableState.value = OfflineCapabilityState(
                stage = OfflineCapabilityStage.VERIFIED,
                result = result,
            )
            OfflineDiagnostics.event(
                "offline_capability_verified",
                traceId,
                JSONObject()
                    .put("backend", result.backend.name)
                    .put("initializationMillis", result.initializationMillis)
                    .put("firstTokenMillis", result.firstTokenMillis)
                    .put("totalMillis", result.totalMillis),
            )
            activeAttemptId = null
            Result.success(result)
        } catch (timeout: kotlinx.coroutines.TimeoutCancellationException) {
            val failedStage = mutableState.value.stage
            mutableState.value = OfflineCapabilityState(
                stage = OfflineCapabilityStage.ERROR,
                errorMessage = "验证超时，请确认设备资源充足后重试",
            )
            OfflineDiagnostics.failure(
                "offline_capability_verification_timed_out",
                traceId,
                timeout,
                JSONObject()
                    .put("stage", failedStage.name)
                    .put("timeoutMillis", OfflineModelSpec.VALIDATION_TIMEOUT_MILLIS)
                    .put(
                        "durationMillis",
                        android.os.SystemClock.elapsedRealtime() - verificationStart,
                    ),
            )
            activeAttemptId = null
            Result.failure(timeout)
        } catch (cancelled: CancellationException) {
            val cancelledStage = mutableState.value.stage
            mutableState.value = OfflineCapabilityState()
            OfflineDiagnostics.event(
                "offline_capability_verification_cancelled",
                traceId,
                JSONObject()
                    .put("stage", cancelledStage.name)
                    .put(
                        "durationMillis",
                        android.os.SystemClock.elapsedRealtime() - verificationStart,
                    ),
            )
            activeAttemptId = null
            throw cancelled
        } catch (error: Throwable) {
            val failedStage = mutableState.value.stage
            val message = error.message?.takeIf(String::isNotBlank) ?: error.javaClass.simpleName
            mutableState.value = OfflineCapabilityState(
                stage = OfflineCapabilityStage.ERROR,
                errorMessage = message,
            )
            OfflineDiagnostics.failure(
                "offline_capability_verification_failed",
                traceId,
                error,
                JSONObject()
                    .put("stage", failedStage.name)
                    .put(
                        "durationMillis",
                        android.os.SystemClock.elapsedRealtime() - verificationStart,
                    ),
            )
            activeAttemptId = null
            Result.failure(error)
        }
    }

    fun disable() {
        checkInitialized()
        if (mutableState.value.inProgress) return
        preferences.disable()
        OfflineDiagnostics.event("offline_mode_disabled", traceId = null)
        mutableState.value = OfflineCapabilityState(
            result = preferences.result.value,
        )
    }

    fun enableVerified(): Boolean {
        checkInitialized()
        if (mutableState.value.inProgress) return false
        val result = preferences.result.value ?: return false
        preferences.enableVerified()
        OfflineDiagnostics.event(
            "verified_offline_mode_enabled",
            traceId = null,
            data = JSONObject().put("backend", result.backend.name),
        )
        mutableState.value = OfflineCapabilityState(
            stage = OfflineCapabilityStage.VERIFIED,
            result = result,
        )
        return true
    }

    private fun checkInitialized() {
        check(::preferences.isInitialized) { "OfflineCapabilityRepository.init must be called first" }
    }
}

private data class ModelSourceMetadata(
    val displayName: String?,
    val length: Long?,
)

private fun modelSourceMetadata(context: Context, uri: Uri): ModelSourceMetadata {
    var displayName: String? = null
    var length: Long? = null
    context.contentResolver.query(
        uri,
        arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE),
        null,
        null,
        null,
    )?.use { cursor ->
        if (cursor.moveToFirst()) {
            val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            val sizeIndex = cursor.getColumnIndex(OpenableColumns.SIZE)
            if (nameIndex >= 0 && !cursor.isNull(nameIndex)) displayName = cursor.getString(nameIndex)
            if (sizeIndex >= 0 && !cursor.isNull(sizeIndex)) length = cursor.getLong(sizeIndex)
        }
    }
    return ModelSourceMetadata(displayName, length)
}

private fun appVersionCode(context: Context): Long {
    val packageInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        context.packageManager.getPackageInfo(
            context.packageName,
            PackageManager.PackageInfoFlags.of(0),
        )
    } else {
        @Suppress("DEPRECATION")
        context.packageManager.getPackageInfo(context.packageName, 0)
    }
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
        packageInfo.longVersionCode
    } else {
        @Suppress("DEPRECATION")
        packageInfo.versionCode.toLong()
    }
}
