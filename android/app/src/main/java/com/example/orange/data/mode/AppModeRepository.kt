package com.example.orange.data.mode

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

object AppModeRepository {
    private lateinit var preferences: SharedPreferences
    private val mutableMode = MutableStateFlow(AppMode.NORMAL)

    val mode: StateFlow<AppMode> = mutableMode.asStateFlow()

    fun init(context: Context) {
        if (::preferences.isInitialized) return
        synchronized(this) {
            if (::preferences.isInitialized) return
            val appContext = context.applicationContext
            preferences = appContext.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)
            mutableMode.value = readOrMigrate(appContext)
        }
    }

    fun current(): AppMode {
        checkInitialized()
        return mutableMode.value
    }

    fun set(mode: AppMode) {
        checkInitialized()
        persistentMode(mode)?.let {
            preferences.edit().putString(KEY_MODE, it.name).apply()
        }
        mutableMode.value = mode
    }

    private fun readOrMigrate(context: Context): AppMode {
        val legacyEnabled = context.getSharedPreferences(
            LEGACY_FILE_NAME,
            Context.MODE_PRIVATE,
        ).getBoolean(LEGACY_ENABLED_KEY, false)
        return resolveInitialMode(preferences.getString(KEY_MODE, null), legacyEnabled).also {
            preferences.edit().putString(KEY_MODE, it.name).apply()
        }
    }

    private fun checkInitialized() {
        check(::preferences.isInitialized) { "AppModeRepository.init must be called first" }
    }

    internal const val FILE_NAME = "app_mode"
    internal const val KEY_MODE = "mode"
    private const val LEGACY_FILE_NAME = "standalone_mode"
    private const val LEGACY_ENABLED_KEY = "enabled"
}

internal fun resolveInitialMode(savedMode: String?, legacyEnabled: Boolean): AppMode =
    savedMode
        ?.let { runCatching { AppMode.valueOf(it) }.getOrNull() }
        ?.takeIf { it != AppMode.OFFLINE }
        ?: if (legacyEnabled) AppMode.STANDALONE else AppMode.NORMAL

internal fun persistentMode(mode: AppMode): AppMode? =
    mode.takeIf { it != AppMode.OFFLINE }
