package com.example.orange.data.contextsync

import java.net.ConnectException
import org.junit.Assert.assertEquals
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

    @Test
    fun reverseActivationCount_requiresConsecutiveConnectFailuresWithPresence() {
        val connectFailure = ForwardProbeResult.ConnectFailed(
            phase = "connect_or_response_headers",
            cause = ConnectException("blocked"),
        )

        val first = ContextSyncConnectionManager.nextConnectFailureCount(
            previous = 0,
            presenceAvailable = true,
            results = listOf(connectFailure),
        )
        val second = ContextSyncConnectionManager.nextConnectFailureCount(
            previous = first,
            presenceAvailable = true,
            results = listOf(connectFailure),
        )

        assertEquals(1, first)
        assertEquals(2, second)
        assertEquals(
            0,
            ContextSyncConnectionManager.nextConnectFailureCount(
                previous = second,
                presenceAvailable = true,
                results = listOf(ForwardProbeResult.HttpFailed(503)),
            ),
        )
        assertEquals(
            0,
            ContextSyncConnectionManager.nextConnectFailureCount(
                previous = second,
                presenceAvailable = false,
                results = listOf(connectFailure),
            ),
        )
    }
}
