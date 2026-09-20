package com.example.orange.data.contextsync

import java.net.InetAddress
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ContextSyncDiscoveryTest {
    @Test
    fun serverIdsMatch_onlyAcceptsPinnedServer() {
        val trusted = "0123456789abcdef0123456789abcdef"

        assertTrue(ContextSyncDiscovery.serverIdsMatch(trusted, trusted))
        assertFalse(
            ContextSyncDiscovery.serverIdsMatch(
                trusted,
                "fedcba9876543210fedcba9876543210",
            ),
        )
        assertFalse(ContextSyncDiscovery.serverIdsMatch(trusted, null))
        assertFalse(ContextSyncDiscovery.serverIdsMatch("invalid", "invalid"))
    }

    @Test
    fun selectTrustedEndpoint_skipsAnotherDeploymentOnSameLan() {
        val trusted = "0123456789abcdef0123456789abcdef"
        val other = ContextSyncEndpoint(
            "http://192.168.1.20:8888",
            "fedcba9876543210fedcba9876543210",
        )
        val own = ContextSyncEndpoint("http://192.168.1.30:8888", trusted)

        assertTrue(
            ContextSyncDiscovery.selectTrustedEndpoint(trusted, listOf(other, own)) === own,
        )
        assertTrue(
            ContextSyncDiscovery.selectTrustedEndpoint(trusted, listOf(other)) == null,
        )
    }

    @Test
    fun allowedLanAddress_acceptsPrivateAndRejectsPublicAddresses() {
        assertTrue(
            ContextSyncDiscovery.isAllowedLanAddress(
                InetAddress.getByAddress(byteArrayOf(192.toByte(), 168.toByte(), 1, 20)),
            ),
        )
        assertTrue(
            ContextSyncDiscovery.isAllowedLanAddress(
                InetAddress.getByAddress(byteArrayOf(10, 0, 0, 5)),
            ),
        )
        assertTrue(
            ContextSyncDiscovery.isAllowedLanAddress(
                InetAddress.getByName("fc00::1234"),
            ),
        )
        assertFalse(
            ContextSyncDiscovery.isAllowedLanAddress(
                InetAddress.getByName("8.8.8.8"),
            ),
        )
        assertFalse(
            ContextSyncDiscovery.isAllowedLanAddress(
                InetAddress.getLoopbackAddress(),
            ),
        )
    }
}
