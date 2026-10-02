package com.example.orange.data.mode

import org.junit.Assert.assertEquals
import org.junit.Test

class OfflineAutoModeControllerTest {
    @Test
    fun networkState_drivesOfflineModeInBothDirections() {
        assertEquals(
            OfflineNetworkAction.ENABLE_OFFLINE,
            offlineNetworkAction(AppMode.NORMAL, internetAvailable = false),
        )
        assertEquals(
            OfflineNetworkAction.ENABLE_OFFLINE,
            offlineNetworkAction(AppMode.STANDALONE, internetAvailable = false),
        )
        assertEquals(
            OfflineNetworkAction.DISABLE_OFFLINE,
            offlineNetworkAction(AppMode.OFFLINE, internetAvailable = true),
        )
        assertEquals(
            OfflineNetworkAction.STAY_OFFLINE,
            offlineNetworkAction(AppMode.OFFLINE, internetAvailable = false),
        )
    }

    @Test
    fun restoredNetwork_returnsToPreviousOnlineMode() {
        assertEquals(AppMode.NORMAL, resolvePreviousOnlineMode(AppMode.NORMAL.name))
        assertEquals(AppMode.STANDALONE, resolvePreviousOnlineMode(AppMode.STANDALONE.name))
        assertEquals(AppMode.NORMAL, resolvePreviousOnlineMode(AppMode.OFFLINE.name))
        assertEquals(AppMode.NORMAL, resolvePreviousOnlineMode("invalid"))
    }
}
