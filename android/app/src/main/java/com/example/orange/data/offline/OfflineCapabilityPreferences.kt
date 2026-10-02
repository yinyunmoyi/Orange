package com.example.orange.data.offline

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

class OfflineCapabilityPreferences(context: Context) {
    private val preferences = context.applicationContext.getSharedPreferences(
        FILE_NAME,
        Context.MODE_PRIVATE,
    )
    private val mutableEnabled = MutableStateFlow(preferences.getBoolean(KEY_ENABLED, false))
    private val mutableResult = MutableStateFlow(readResult(preferences))
    private val listener = SharedPreferences.OnSharedPreferenceChangeListener { prefs, _ ->
        mutableEnabled.value = prefs.getBoolean(KEY_ENABLED, false)
        mutableResult.value = readResult(prefs)
    }

    val enabled: StateFlow<Boolean> = mutableEnabled.asStateFlow()
    val result: StateFlow<OfflineCapabilityResult?> = mutableResult.asStateFlow()

    init {
        preferences.registerOnSharedPreferenceChangeListener(listener)
    }

    fun cachedFingerprint(): OfflineModelFingerprint? = mutableResult.value?.fingerprint

    fun save(result: OfflineCapabilityResult) {
        preferences.edit()
            .putBoolean(KEY_ENABLED, true)
            .putString(KEY_MODEL_PATH, result.fingerprint.path)
            .putLong(KEY_MODEL_LENGTH, result.fingerprint.length)
            .putLong(KEY_MODEL_LAST_MODIFIED, result.fingerprint.lastModified)
            .putString(KEY_MODEL_SHA256, result.fingerprint.sha256)
            .putLong(KEY_APP_VERSION_CODE, result.appVersionCode)
            .putString(KEY_BACKEND, result.backend.name)
            .putString(KEY_REPLY, result.reply)
            .putLong(KEY_INITIALIZATION_MILLIS, result.initializationMillis)
            .putLong(KEY_FIRST_TOKEN_MILLIS, result.firstTokenMillis)
            .putLong(KEY_TOTAL_MILLIS, result.totalMillis)
            .apply()
        mutableEnabled.value = true
        mutableResult.value = result
    }

    fun disable() {
        preferences.edit().putBoolean(KEY_ENABLED, false).apply()
        mutableEnabled.value = false
    }

    fun enableVerified() {
        check(mutableResult.value != null) { "No verified offline capability is available" }
        preferences.edit().putBoolean(KEY_ENABLED, true).apply()
        mutableEnabled.value = true
    }

    fun invalidateIfStale(modelFile: FileMetadata?, appVersionCode: Long): Boolean {
        val saved = mutableResult.value ?: return false
        if (saved.isCurrent(modelFile, appVersionCode)) return false
        preferences.edit().clear().apply()
        mutableEnabled.value = false
        mutableResult.value = null
        return true
    }

    internal companion object {
        const val FILE_NAME = "offline_capability"
        private const val KEY_ENABLED = "enabled"
        private const val KEY_MODEL_PATH = "model_path"
        private const val KEY_MODEL_LENGTH = "model_length"
        private const val KEY_MODEL_LAST_MODIFIED = "model_last_modified"
        private const val KEY_MODEL_SHA256 = "model_sha256"
        private const val KEY_APP_VERSION_CODE = "app_version_code"
        private const val KEY_BACKEND = "backend"
        private const val KEY_REPLY = "reply"
        private const val KEY_INITIALIZATION_MILLIS = "initialization_millis"
        private const val KEY_FIRST_TOKEN_MILLIS = "first_token_millis"
        private const val KEY_TOTAL_MILLIS = "total_millis"

        private fun readResult(preferences: SharedPreferences): OfflineCapabilityResult? {
            val path = preferences.getString(KEY_MODEL_PATH, null) ?: return null
            val sha256 = preferences.getString(KEY_MODEL_SHA256, null) ?: return null
            val backend = preferences.getString(KEY_BACKEND, null)
                ?.let { runCatching { OfflineBackend.valueOf(it) }.getOrNull() }
                ?: return null
            return OfflineCapabilityResult(
                fingerprint = OfflineModelFingerprint(
                    path = path,
                    length = preferences.getLong(KEY_MODEL_LENGTH, -1L),
                    lastModified = preferences.getLong(KEY_MODEL_LAST_MODIFIED, -1L),
                    sha256 = sha256,
                ),
                appVersionCode = preferences.getLong(KEY_APP_VERSION_CODE, -1L),
                backend = backend,
                reply = preferences.getString(KEY_REPLY, "").orEmpty(),
                initializationMillis = preferences.getLong(KEY_INITIALIZATION_MILLIS, -1L),
                firstTokenMillis = preferences.getLong(KEY_FIRST_TOKEN_MILLIS, -1L),
                totalMillis = preferences.getLong(KEY_TOTAL_MILLIS, -1L),
            )
        }
    }
}

data class FileMetadata(
    val path: String,
    val length: Long,
    val lastModified: Long,
)

internal fun OfflineCapabilityResult.isCurrent(
    modelFile: FileMetadata?,
    currentAppVersionCode: Long,
): Boolean = modelFile != null &&
    appVersionCode == currentAppVersionCode &&
    fingerprint.path == modelFile.path &&
    fingerprint.length == modelFile.length &&
    fingerprint.lastModified == modelFile.lastModified &&
    fingerprint.sha256 == OfflineModelSpec.SHA256
