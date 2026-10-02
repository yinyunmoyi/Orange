package com.example.orange.ui.bookshelf

import com.example.orange.data.mode.OfflineAutoModeState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BookshelfScreenTest {
    @Test
    fun readerOfflineStatus_usesSpecifiedMessages() {
        assertEquals(
            "离线模式初始化中",
            status(OfflineAutoModeState.INITIALIZING),
        )
        assertEquals(
            "离线模式初始化失败，本地模型配置不正确",
            status(OfflineAutoModeState.FAILED),
        )
        assertEquals(
            "当前已进入离线模式，可由本地大模型提供查询能力",
            status(OfflineAutoModeState.READY),
        )
    }

    @Test
    fun readerOfflineStatus_isHiddenWhenNetworkIsAvailable() {
        assertFalse(shouldShowOfflineCapabilityBar(OfflineAutoModeState.IDLE))
        assertFalse(shouldShowOfflineCapabilityBar(OfflineAutoModeState.DISABLING))
        assertTrue(shouldShowOfflineCapabilityBar(OfflineAutoModeState.INITIALIZING))
        assertTrue(shouldShowOfflineCapabilityBar(OfflineAutoModeState.READY))
        assertTrue(shouldShowOfflineCapabilityBar(OfflineAutoModeState.FAILED))
    }

    @Test
    fun readerOfflineConfiguration_isOnlyAvailableAfterFailure() {
        assertFalse(shouldShowOfflineConfigureButton(OfflineAutoModeState.IDLE))
        assertFalse(shouldShowOfflineConfigureButton(OfflineAutoModeState.INITIALIZING))
        assertFalse(shouldShowOfflineConfigureButton(OfflineAutoModeState.READY))
        assertFalse(shouldShowOfflineConfigureButton(OfflineAutoModeState.DISABLING))
        assertTrue(shouldShowOfflineConfigureButton(OfflineAutoModeState.FAILED))
    }

    private fun status(state: OfflineAutoModeState): String =
        offlineCapabilityStatusText(state)
}
