package com.example.orange.data.word

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class WordServiceConfigTest {
    @Test
    fun `https uses default port and removes trailing slash`() {
        val endpoint = parseWordServiceEndpoint(" https://example.com/ ")

        assertEquals("https", endpoint.scheme)
        assertEquals("example.com", endpoint.host)
        assertEquals(443, endpoint.port)
        assertEquals("https://example.com", endpoint.origin)
    }

    @Test
    fun `http uses default port`() {
        val endpoint = parseWordServiceEndpoint("http://localhost")

        assertEquals(80, endpoint.port)
        assertEquals("http://localhost", endpoint.origin)
    }

    @Test
    fun `explicit port is preserved`() {
        val endpoint = parseWordServiceEndpoint("http://10.0.2.2:8888")

        assertEquals(8888, endpoint.port)
        assertEquals("http://10.0.2.2:8888", endpoint.origin)
    }

    @Test
    fun `path and unsupported scheme are rejected`() {
        assertThrows(IllegalArgumentException::class.java) {
            parseWordServiceEndpoint("https://example.com/api")
        }
        assertThrows(IllegalArgumentException::class.java) {
            parseWordServiceEndpoint("ftp://example.com")
        }
        assertThrows(IllegalArgumentException::class.java) {
            parseWordServiceEndpoint("http://example.com:0")
        }
    }
}
