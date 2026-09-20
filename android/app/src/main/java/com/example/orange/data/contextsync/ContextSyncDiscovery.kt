package com.example.orange.data.contextsync

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import com.example.orange.data.word.WordServiceConfig
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.util.ArrayDeque
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.coroutines.resume

data class ContextSyncEndpoint(
    val origin: String,
    val serverId: String,
)

class ContextSyncDiscovery(private val context: Context) {
    private val preferences = ContextSyncPreferences(context)

    suspend fun discover(): ContextSyncEndpoint? {
        val trustedServerId = trustedServerId() ?: return null
        val cachedOrigin = preferences.localOrigin
        if (cachedOrigin != null) {
            val pingId = ContextVideoSyncApi.ping(cachedOrigin).getOrNull()
            if (serverIdsMatch(trustedServerId, pingId)) {
                return ContextSyncEndpoint(cachedOrigin, trustedServerId)
            }
            preferences.clearEndpoint()
        }

        val candidate = discoverWithNsd(trustedServerId) ?: return null
        val pingId = ContextVideoSyncApi.ping(candidate.origin).getOrNull()
        if (!serverIdsMatch(trustedServerId, pingId)) return null
        preferences.saveEndpoint(candidate.origin, trustedServerId)
        return candidate
    }

    private suspend fun trustedServerId(): String? {
        preferences.serverId?.let { return it }
        val publicServerId = ContextVideoSyncApi.ping(WordServiceConfig.origin).getOrNull()
            ?.takeIf(::isValidServerId)
            ?: return null
        return preferences.trustServerIdIfAbsent(publicServerId)
    }

    private suspend fun discoverWithNsd(trustedServerId: String): ContextSyncEndpoint? {
        val wifi = context.applicationContext.getSystemService(WifiManager::class.java)
        val lock = wifi?.createMulticastLock("orange-context-sync")?.apply {
            setReferenceCounted(false)
            acquire()
        }
        return try {
            withTimeoutOrNull(3_000) { awaitNsdCandidate(trustedServerId) }
        } finally {
            if (lock?.isHeld == true) lock.release()
        }
    }

    @Suppress("DEPRECATION")
    private suspend fun awaitNsdCandidate(trustedServerId: String): ContextSyncEndpoint? =
        suspendCancellableCoroutine { continuation ->
            val manager = context.getSystemService(NsdManager::class.java)
            val completed = AtomicBoolean(false)
            val pending = ArrayDeque<NsdServiceInfo>()
            val resolutionLock = Any()
            var resolving = false
            lateinit var listener: NsdManager.DiscoveryListener
            lateinit var resolveNext: () -> Unit

            fun finish(value: ContextSyncEndpoint?) {
                if (!completed.compareAndSet(false, true)) return
                runCatching { manager.stopServiceDiscovery(listener) }
                if (continuation.isActive) continuation.resume(value)
            }

            resolveNext = resolveNext@{
                val service = synchronized(resolutionLock) {
                    if (completed.get() || resolving || pending.isEmpty()) {
                        null
                    } else {
                        resolving = true
                        pending.removeFirst()
                    }
                } ?: return@resolveNext
                val started = runCatching {
                    manager.resolveService(
                        service,
                        object : NsdManager.ResolveListener {
                            override fun onResolveFailed(
                                serviceInfo: NsdServiceInfo,
                                errorCode: Int,
                            ) {
                                synchronized(resolutionLock) { resolving = false }
                                resolveNext()
                            }

                            override fun onServiceResolved(info: NsdServiceInfo) {
                                val endpoint = selectTrustedEndpoint(
                                    trustedServerId,
                                    endpointsFromService(info),
                                )
                                if (endpoint != null) {
                                    finish(endpoint)
                                    return
                                }
                                synchronized(resolutionLock) { resolving = false }
                                resolveNext()
                            }
                        },
                    )
                }.isSuccess
                if (!started) {
                    synchronized(resolutionLock) { resolving = false }
                    resolveNext()
                }
            }

            listener = object : NsdManager.DiscoveryListener {
                override fun onDiscoveryStarted(serviceType: String) = Unit
                override fun onServiceLost(serviceInfo: NsdServiceInfo) = Unit
                override fun onDiscoveryStopped(serviceType: String) = Unit
                override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                    finish(null)
                }

                override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) = Unit

                override fun onServiceFound(serviceInfo: NsdServiceInfo) {
                    if (serviceInfo.serviceType != SERVICE_TYPE || completed.get()) return
                    synchronized(resolutionLock) { pending.addLast(serviceInfo) }
                    resolveNext()
                }
            }
            continuation.invokeOnCancellation {
                if (completed.compareAndSet(false, true)) {
                    runCatching { manager.stopServiceDiscovery(listener) }
                }
            }
            manager.discoverServices(SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, listener)
        }

    @Suppress("DEPRECATION")
    private fun endpointsFromService(info: NsdServiceInfo): List<ContextSyncEndpoint> {
        val serverId = info.attributes["id"]?.toString(Charsets.UTF_8)?.trim().orEmpty()
        val api = info.attributes["api"]?.toString(Charsets.UTF_8)?.trim()
        if (serverId.isEmpty() || api != "1" || info.port !in 1..65535) return emptyList()
        val addresses = if (Build.VERSION.SDK_INT >= 34) {
            info.hostAddresses
        } else {
            listOfNotNull(info.host)
        }
        return addresses
            .filter(::isAllowedLanAddress)
            .sortedBy { if (it is Inet4Address) 0 else 1 }
            .map { address ->
                val host = if (address is Inet6Address) {
                    "[${address.hostAddress?.replace("%", "%25")}]"
                } else {
                    address.hostAddress
                }
                ContextSyncEndpoint("http://$host:${info.port}", serverId)
            }
            .distinctBy { it.origin }
    }

    companion object {
        const val SERVICE_TYPE = "_orange-context._tcp."

        internal fun isValidServerId(serverId: String): Boolean =
            SERVER_ID_PATTERN.matches(serverId)

        internal fun serverIdsMatch(expected: String, actual: String?): Boolean =
            isValidServerId(expected) && expected == actual

        internal fun selectTrustedEndpoint(
            trustedServerId: String,
            candidates: List<ContextSyncEndpoint>,
        ): ContextSyncEndpoint? =
            candidates.firstOrNull {
                serverIdsMatch(trustedServerId, it.serverId)
            }

        internal fun isAllowedLanAddress(address: InetAddress): Boolean {
            if (address.isLoopbackAddress || address.isAnyLocalAddress || address.isMulticastAddress) {
                return false
            }
            if (address is Inet4Address) return address.isSiteLocalAddress || address.isLinkLocalAddress
            if (address !is Inet6Address) return false
            val first = address.address.firstOrNull()?.toInt()?.and(0xff) ?: return false
            val uniqueLocal = first and 0xfe == 0xfc
            return uniqueLocal || address.isLinkLocalAddress || address.isSiteLocalAddress
        }

        private val SERVER_ID_PATTERN = Regex("^[a-f0-9]{32}$")
    }
}
