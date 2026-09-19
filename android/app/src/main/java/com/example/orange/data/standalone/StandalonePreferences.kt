package com.example.orange.data.standalone

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

class StandalonePreferences(context: Context) {
    private val preferences = context.applicationContext.getSharedPreferences(
        FILE_NAME,
        Context.MODE_PRIVATE,
    )
    private val mutableEnabled = MutableStateFlow(preferences.getBoolean(KEY_ENABLED, false))
    private val listener = SharedPreferences.OnSharedPreferenceChangeListener { prefs, key ->
        if (key == KEY_ENABLED) mutableEnabled.value = prefs.getBoolean(KEY_ENABLED, false)
    }

    val enabled: StateFlow<Boolean> = mutableEnabled.asStateFlow()

    init {
        preferences.registerOnSharedPreferenceChangeListener(listener)
    }

    fun isEnabled(): Boolean = mutableEnabled.value

    fun setEnabled(enabled: Boolean) {
        preferences.edit().putBoolean(KEY_ENABLED, enabled).apply()
        mutableEnabled.value = enabled
    }

    var lastSyncError: String?
        get() = preferences.getString(KEY_LAST_SYNC_ERROR, null)
        set(value) {
            preferences.edit().apply {
                if (value == null) remove(KEY_LAST_SYNC_ERROR) else putString(KEY_LAST_SYNC_ERROR, value)
            }.apply()
        }

    var lastSyncAt: Long
        get() = preferences.getLong(KEY_LAST_SYNC_AT, 0L)
        set(value) {
            preferences.edit().putLong(KEY_LAST_SYNC_AT, value).apply()
        }

    private companion object {
        const val FILE_NAME = "standalone_mode"
        const val KEY_ENABLED = "enabled"
        const val KEY_LAST_SYNC_ERROR = "last_sync_error"
        const val KEY_LAST_SYNC_AT = "last_sync_at"
    }
}
