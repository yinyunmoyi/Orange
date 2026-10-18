package com.example.orange.data.contextsync

import android.content.Context

class ContextSyncPreferences(context: Context) {
    private val preferences = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)

    var localOrigin: String?
        get() = preferences.getString(KEY_ORIGIN, null)
        set(value) {
            preferences.edit().putString(KEY_ORIGIN, value).apply()
        }

    val serverId: String?
        get() = preferences.getString(KEY_TRUSTED_SERVER_ID, null)

    var permissionAsked: Boolean
        get() = preferences.getBoolean(KEY_PERMISSION_ASKED, false)
        set(value) {
            preferences.edit().putBoolean(KEY_PERMISSION_ASKED, value).apply()
        }

    @Synchronized
    fun saveEndpoint(origin: String, serverId: String): Boolean {
        check(serverId == this.serverId) { "cannot cache an untrusted context sync server" }
        if (localOrigin == origin) return false
        preferences.edit()
            .putString(KEY_ORIGIN, origin)
            .commit()
        return true
    }

    @Synchronized
    fun clearEndpoint(expectedOrigin: String? = null): Boolean {
        val current = localOrigin ?: return false
        if (expectedOrigin != null && current != expectedOrigin) return false
        preferences.edit().remove(KEY_ORIGIN).commit()
        return true
    }

    @Synchronized
    fun trustServerIdIfAbsent(serverId: String): String {
        val existing = this.serverId
        if (existing != null) return existing
        preferences.edit()
            .putString(KEY_TRUSTED_SERVER_ID, serverId)
            .remove(LEGACY_KEY_SERVER_ID)
            .commit()
        return serverId
    }

    private companion object {
        const val NAME = "context_video_sync"
        const val KEY_ORIGIN = "local_origin"
        const val KEY_TRUSTED_SERVER_ID = "trusted_server_id_v2"
        const val LEGACY_KEY_SERVER_ID = "server_id"
        const val KEY_PERMISSION_ASKED = "local_network_permission_asked"
    }
}
