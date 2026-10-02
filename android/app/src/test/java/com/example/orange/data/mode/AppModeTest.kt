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
        assertEquals(AppMode.OFFLINE, resolveInitialMode("OFFLINE", legacyEnabled = true))
    }

    @Test
    fun localData_isUsedOnlyOutsideNormalMode() {
        assertEquals(false, AppMode.NORMAL.usesLocalData)
        assertEquals(true, AppMode.STANDALONE.usesLocalData)
        assertEquals(true, AppMode.OFFLINE.usesLocalData)
    }
}
