package com.example.orange.data.contextsync

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ContextSyncConnectionManagerTest {
    private val trustedServerId = "0123456789abcdef0123456789abcdef"
    private val endpoint = ContextSyncEndpoint(
        origin = "http://192.168.1.6:8888",
        serverId = trustedServerId,
    )

    @Test
    fun generationGuardRejectsStalePausedAndOfflineResults() {
        assertTrue(
            ContextSyncConnectionManager.isGenerationCurrent(
                currentGeneration = 4,
                candidateGeneration = 4,
                paused = false,
                networkAllowed = true,
            ),
        )
        assertFalse(ContextSyncConnectionManager.isGenerationCurrent(5, 4, false, true))
        assertFalse(ContextSyncConnectionManager.isGenerationCurrent(4, 4, true, true))
        assertFalse(ContextSyncConnectionManager.isGenerationCurrent(4, 4, false, false))
    }

    @Test
    fun endpointAcceptanceRequiresCurrentGenerationAndPinnedIdentity() {
        assertTrue(
            ContextSyncConnectionManager.shouldAcceptEndpoint(
                currentGeneration = 8,
                candidateGeneration = 8,
                paused = false,
                networkAllowed = true,
                trustedServerId = trustedServerId,
                candidate = endpoint,
            ),
        )
        assertFalse(
            ContextSyncConnectionManager.shouldAcceptEndpoint(
                currentGeneration = 9,
                candidateGeneration = 8,
                paused = false,
                networkAllowed = true,
                trustedServerId = trustedServerId,
                candidate = endpoint,
            ),
        )
        assertFalse(
            ContextSyncConnectionManager.shouldAcceptEndpoint(
                currentGeneration = 8,
                candidateGeneration = 8,
                paused = false,
                networkAllowed = true,
                trustedServerId = "fedcba9876543210fedcba9876543210",
                candidate = endpoint,
            ),
        )
    }
}
