package com.example.orange.data.network

import com.example.orange.data.mode.AppMode
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLConnection
import java.net.URLStreamHandler
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class NetworkAccessPolicyTest {
    @After
    fun reset() {
        NetworkAccessPolicy.setOffline(false)
    }

    @Test
    fun offlineMode_rejectsConnectionBeforeOpeningIt() {
        var opens = 0
        val url = fakeUrl {
            opens++
            FakeConnection(it)
        }
        NetworkAccessPolicy.initialize(AppMode.OFFLINE)

        assertThrows(NetworkBlockedException::class.java) {
            NetworkAccessPolicy.openConnection(url)
        }
        assertEquals(0, opens)
    }

    @Test
    fun enteringOffline_disconnectsTrackedConnections() {
        lateinit var connection: FakeConnection
        val url = fakeUrl { connectionUrl ->
            FakeConnection(connectionUrl).also { connection = it }
        }
        NetworkAccessPolicy.initialize(AppMode.NORMAL)
        NetworkAccessPolicy.openConnection(url)

        NetworkAccessPolicy.setOffline(true)

        assertTrue(connection.disconnected)
        assertEquals(0, NetworkAccessPolicy.activeConnectionCount())
    }

    @Test
    fun release_removesConnectionFromTracking() {
        val connection = NetworkAccessPolicy.openConnection(fakeUrl(::FakeConnection))

        NetworkAccessPolicy.release(connection)

        assertEquals(0, NetworkAccessPolicy.activeConnectionCount())
    }

    private fun fakeUrl(factory: (URL) -> URLConnection): URL =
        URL(null, "test://orange", object : URLStreamHandler() {
            override fun openConnection(url: URL): URLConnection = factory(url)
        })

    private class FakeConnection(url: URL) : HttpURLConnection(url) {
        var disconnected = false

        override fun connect() = Unit
        override fun usingProxy(): Boolean = false
        override fun disconnect() {
            disconnected = true
        }
    }
}
