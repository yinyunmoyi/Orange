package com.example.orange.data.contextsync

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import android.os.SystemClock
import com.example.orange.data.logging.AppLog as Log
import com.example.orange.data.word.WordServiceConfig
import com.example.orange.data.network.NetworkAccessPolicy
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import org.json.JSONObject
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.util.ArrayDeque
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.resume

data class ContextSyncEndpoint(
    val origin: String,
    val serverId: String,
)

data class ContextSyncPresence(
    val serverId: String,
    val addresses: List<InetAddress>,
    val port: Int,
) {
    fun endpoints(): List<ContextSyncEndpoint> = addresses
        .filter { ContextSyncDiscovery.isAllowedLanAddress(it) }
        .sortedBy { if (it is Inet4Address) 0 else 1 }
        .mapNotNull { address ->
            val rawHost = address.hostAddress ?: return@mapNotNull null
            val host = if (address is Inet6Address) {
                "[${rawHost.replace("%", "%25")}]"
            } else {
                rawHost
            }
            ContextSyncEndpoint("http://$host:$port", serverId)
        }
        .distinctBy(ContextSyncEndpoint::origin)
}

class ContextSyncDiscovery(private val context: Context) {
    private val preferences = ContextSyncPreferences(context)

    suspend fun discover(): ContextSyncEndpoint? {
        val endpoint = discoverCandidate() ?: return null
        preferences.saveEndpoint(endpoint.origin, endpoint.serverId)
        return endpoint
    }

    internal suspend fun discoverCandidate(): ContextSyncEndpoint? {
        NetworkAccessPolicy.requireAllowed()
        val started = SystemClock.elapsedRealtime()
        if (shouldLogDiscoveryStarted(started)) {
            Log.event(
                TAG,
                "discovery_started",
                JSONObject().put("cached", preferences.localOrigin != null),
            )
        }
        val trustedServerId = trustedServerId()
        if (trustedServerId == null) {
            logRejected("trusted_id_unavailable", null, started)
            return null
        }
        val cachedOrigin = preferences.localOrigin
        if (cachedOrigin != null) {
            val pingId = ContextVideoSyncApi.ping(cachedOrigin).getOrNull()
            if (serverIdsMatch(trustedServerId, pingId)) {
                return ContextSyncEndpoint(cachedOrigin, trustedServerId)
            }
            logRejected("cached_ping_failed", cachedOrigin, started)
        }

        val presence = discoverPresence(trustedServerId)
        if (presence == null) {
            val now = SystemClock.elapsedRealtime()
            if (shouldLogTimeout(now)) {
                Log.event(
                    TAG,
                    "discovery_timeout",
                    JSONObject().put("durationMs", now - started),
                )
            }
            return null
        }
        for (candidate in presence.endpoints()) {
            when (ContextVideoSyncApi.probe(candidate.origin, trustedServerId)) {
                is ForwardProbeResult.Verified -> {
                    Log.event(
                        TAG,
                        "candidate_verified",
                        JSONObject()
                            .put("origin", candidate.origin)
                            .put("serverIdPrefix", trustedServerId.take(8))
                            .put("durationMs", SystemClock.elapsedRealtime() - started),
                    )
                    return candidate
                }
                else -> logRejected("candidate_ping_failed", candidate.origin, started)
            }
        }
        return null
    }

    private fun logRejected(reason: String, origin: String?, started: Long) {
        if (!shouldLogRejected(SystemClock.elapsedRealtime())) return
        Log.event(
            TAG,
            "candidate_rejected",
            JSONObject()
                .put("reason", reason)
                .put("origin", origin ?: "")
                .put("durationMs", SystemClock.elapsedRealtime() - started),
        )
    }

    private suspend fun trustedServerId(): String? {
        preferences.serverId?.let { return it }
        val publicServerId = ContextVideoSyncApi.ping(WordServiceConfig.origin).getOrNull()
            ?.takeIf(::isValidServerId)
            ?: return null
        return preferences.trustServerIdIfAbsent(publicServerId)
    }

    internal suspend fun discoverPresence(): ContextSyncPresence? {
        NetworkAccessPolicy.requireAllowed()
        val trustedServerId = trustedServerId() ?: return null
        return discoverPresence(trustedServerId)
    }

    private suspend fun discoverPresence(trustedServerId: String): ContextSyncPresence? {
        NetworkAccessPolicy.requireAllowed()
        val wifi = context.applicationContext.getSystemService(WifiManager::class.java)
        val lock = wifi?.createMulticastLock("orange-context-sync")?.apply {
            setReferenceCounted(false)
            acquire()
        }
        return try {
            withTimeoutOrNull(3_000) { awaitNsdPresence(trustedServerId) }
        } finally {
            if (lock?.isHeld == true) lock.release()
        }
    }

    @Suppress("DEPRECATION")
    private suspend fun awaitNsdPresence(trustedServerId: String): ContextSyncPresence? =
        suspendCancellableCoroutine { continuation ->
            val manager = context.getSystemService(NsdManager::class.java)
            val completed = AtomicBoolean(false)
            val pending = ArrayDeque<NsdServiceInfo>()
            val resolutionLock = Any()
            var resolving = false
            lateinit var listener: NsdManager.DiscoveryListener
            lateinit var resolveNext: () -> Unit

            fun finish(value: ContextSyncPresence?) {
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
                                val presence = presenceFromService(
                                    trustedServerId,
                                    info,
                                )
                                if (presence != null) {
                                    finish(presence)
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
                    if (shouldLogServiceFound(SystemClock.elapsedRealtime())) {
                        Log.event(
                            TAG,
                            "service_found",
                            JSONObject().put("serviceName", serviceInfo.serviceName.take(100)),
                        )
                    }
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
    private fun presenceFromService(
        trustedServerId: String,
        info: NsdServiceInfo,
    ): ContextSyncPresence? {
        val serverId = info.attributes["id"]?.toString(Charsets.UTF_8)?.trim().orEmpty()
        val api = info.attributes["api"]?.toString(Charsets.UTF_8)?.trim()
        if (
            !serverIdsMatch(trustedServerId, serverId) ||
            api != "1" ||
            info.port !in 1..65535
        ) {
            return null
        }
        val addresses = if (Build.VERSION.SDK_INT >= 34) {
            info.hostAddresses
        } else {
            listOfNotNull(info.host)
        }
        val allowed = addresses
            .filter(::isAllowedLanAddress)
            .sortedBy { if (it is Inet4Address) 0 else 1 }
            .distinctBy { it.hostAddress }
        if (allowed.isEmpty()) return null
        return ContextSyncPresence(serverId, allowed, info.port)
    }

    companion object {
        const val SERVICE_TYPE = "_orange-context._tcp."
        private const val TAG = "LanDiscovery"
        private const val TIMEOUT_LOG_INTERVAL_MS = 60_000L
        private val lastTimeoutLogAt = AtomicLong(Long.MIN_VALUE)
        private val lastServiceFoundLogAt = AtomicLong(Long.MIN_VALUE)
        private val lastDiscoveryStartedLogAt = AtomicLong(Long.MIN_VALUE)
        private val lastRejectedLogAt = AtomicLong(Long.MIN_VALUE)

        private fun shouldLogTimeout(now: Long): Boolean {
            return shouldLog(now, lastTimeoutLogAt)
        }

        private fun shouldLogDiscoveryStarted(now: Long): Boolean {
            return shouldLog(now, lastDiscoveryStartedLogAt)
        }

        private fun shouldLogServiceFound(now: Long): Boolean {
            return shouldLog(now, lastServiceFoundLogAt)
        }

        private fun shouldLogRejected(now: Long): Boolean {
            return shouldLog(now, lastRejectedLogAt)
        }

        private fun shouldLog(now: Long, lastLogAt: AtomicLong): Boolean {
            while (true) {
                val previous = lastLogAt.get()
                if (previous != Long.MIN_VALUE && now - previous < TIMEOUT_LOG_INTERVAL_MS) {
                    return false
                }
                if (lastLogAt.compareAndSet(previous, now)) return true
            }
        }

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
