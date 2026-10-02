package com.example.orange.data.network

import com.example.orange.data.mode.AppMode
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean

object NetworkAccessPolicy {
    private val blocked = AtomicBoolean(false)
    private val activeConnections = ConcurrentHashMap.newKeySet<HttpURLConnection>()

    fun initialize(mode: AppMode) {
        setOffline(mode == AppMode.OFFLINE)
    }

    fun isAllowed(): Boolean = !blocked.get()

    fun requireAllowed() {
        if (blocked.get()) throw NetworkBlockedException()
    }

    fun openConnection(url: URL): HttpURLConnection {
        requireAllowed()
        val connection = url.openConnection() as HttpURLConnection
        if (blocked.get()) {
            connection.disconnect()
            throw NetworkBlockedException()
        }
        activeConnections += connection
        if (blocked.get()) {
            if (activeConnections.remove(connection)) connection.disconnect()
            throw NetworkBlockedException()
        }
        return connection
    }

    fun release(connection: HttpURLConnection) {
        activeConnections.remove(connection)
        connection.disconnect()
    }

    fun setOffline(offline: Boolean) {
        blocked.set(offline)
        if (!offline) return
        activeConnections.toList().forEach { connection ->
            if (activeConnections.remove(connection)) {
                runCatching { connection.disconnect() }
            }
        }
    }

    internal fun activeConnectionCount(): Int = activeConnections.size
}
