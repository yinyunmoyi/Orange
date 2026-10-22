package com.example.orange.data.mode

import org.junit.Assert.assertEquals
import org.junit.Test

class AppModeTest {
    @Test
    fun legacyStandaloneEnabled_migratesToStandalone() {
        assertEquals(AppMode.STANDALONE, resolveInitialMode(null, legacyEnabled = true))
    }

    @Test
    fun legacyStandaloneDisabled_migratesToNormal() {
        assertEquals(AppMode.NORMAL, resolveInitialMode(null, legacyEnabled = false))
    }

    @Test
    fun savedMode_winsOverLegacyPreference() {
        assertEquals(AppMode.NORMAL, resolveInitialMode("NORMAL", legacyEnabled = true))
    }

    @Test
    fun savedOfflineMode_isTreatedAsUnknownAndStartsOnline() {
        assertEquals(AppMode.NORMAL, resolveInitialMode("OFFLINE", legacyEnabled = false))
        assertEquals(AppMode.STANDALONE, resolveInitialMode("OFFLINE", legacyEnabled = true))
    }

    @Test
    fun offlineMode_isRuntimeOnly() {
        assertEquals(AppMode.NORMAL, persistentMode(AppMode.NORMAL))
        assertEquals(AppMode.STANDALONE, persistentMode(AppMode.STANDALONE))
        assertEquals(null, persistentMode(AppMode.OFFLINE))
    }

    @Test
    fun localData_isUsedOnlyOutsideNormalMode() {
        assertEquals(false, AppMode.NORMAL.usesLocalData)
        assertEquals(true, AppMode.STANDALONE.usesLocalData)
        assertEquals(true, AppMode.OFFLINE.usesLocalData)
    }
}
