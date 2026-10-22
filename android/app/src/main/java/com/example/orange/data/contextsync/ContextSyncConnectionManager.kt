package com.example.orange.data.contextsync

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.os.Build
import androidx.core.content.ContextCompat
import com.example.orange.data.audiosync.AudioSyncScheduler
import com.example.orange.data.logging.AppLog
import com.example.orange.data.mode.AppMode
import com.example.orange.data.mode.AppModeRepository
import com.example.orange.data.network.NetworkAccessPolicy
import com.example.orange.data.reverselan.ReverseLanManager
import com.example.orange.data.standalone.StandaloneSyncScheduler
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject

object ContextSyncConnectionManager {
    private const val TAG = "LanDiscovery"
    private const val NETWORK_DEBOUNCE_MS = 300L
    private const val HEALTH_INTERVAL_MS = 3_000L

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val refreshMutex = Mutex()
    private val callbackRegistered = AtomicBoolean(false)
    private val generation = AtomicLong(0L)
    private val stateLock = Any()

    @Volatile
    private var applicationContext: Context? = null

    @Volatile
    private var paused = true

    private var refreshJob: Job? = null
    private var monitorJob: Job? = null
    private var consecutiveConnectFailures = 0
    private var consecutiveForwardSuccesses = 0

    @Volatile
    private var trustedPresence: ContextSyncPresence? = null

    fun start(context: Context) {
        val appContext = context.applicationContext
        applicationContext = appContext
        if (!NetworkAccessPolicy.isAllowed() || !hasLocalNetworkPermission(appContext)) return

        val resumed = synchronized(stateLock) {
            val changed = paused
            paused = false
            registerNetworkCallback(appContext)
            if (monitorJob?.isActive != true) {
                monitorJob = scope.launch {
                    while (isActive) {
                        delay(HEALTH_INTERVAL_MS)
                        triggerRefresh("health_check", invalidate = false)
                    }
                }
            }
            changed
        }
        if (resumed) {
            AppLog.event(TAG, "monitor_resumed")
        }
        triggerRefresh("start", invalidate = true)
    }

    fun pause() {
        val didPause = synchronized(stateLock) {
            if (paused) return@synchronized false
            paused = true
            generation.incrementAndGet()
            refreshJob?.cancel()
            refreshJob = null
            monitorJob?.cancel()
            monitorJob = null
            true
        }
        if (didPause) {
            AppLog.event(TAG, "monitor_paused")
            scope.launch { ReverseLanManager.stopAndAwait("connection_manager_paused") }
        }
    }

    suspend fun resolveNow(context: Context): ContextSyncEndpoint? {
        applicationContext = context.applicationContext
        if (paused || !NetworkAccessPolicy.isAllowed() ||
            !hasLocalNetworkPermission(context) || !isEligibleWifi(context)
        ) {
            return null
        }
        return refresh(generation.get(), "worker")
    }

    private fun registerNetworkCallback(context: Context) {
        if (!callbackRegistered.compareAndSet(false, true)) return
        val connectivity = context.getSystemService(ConnectivityManager::class.java)
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            .build()
        connectivity.registerNetworkCallback(
            request,
            object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) = onNetworkChanged("available")

                override fun onLost(network: Network) = onNetworkChanged("lost")

                override fun onCapabilitiesChanged(
                    network: Network,
                    capabilities: NetworkCapabilities,
                ) = onNetworkChanged("capabilities")

                override fun onLinkPropertiesChanged(
                    network: Network,
                    linkProperties: LinkProperties,
                ) = onNetworkChanged("link_properties")
            },
        )
    }

    private fun onNetworkChanged(reason: String) {
        AppLog.event(
            TAG,
            "network_changed",
            JSONObject().put("reason", reason).put("generation", generation.get() + 1),
        )
        triggerRefresh("network_$reason", invalidate = true)
    }

    private fun triggerRefresh(reason: String, invalidate: Boolean) {
        val targetGeneration = if (invalidate) generation.incrementAndGet() else generation.get()
        synchronized(stateLock) {
            if (paused) return
            if (!invalidate && refreshJob?.isActive == true) return
            if (invalidate) refreshJob?.cancel()
            refreshJob = scope.launch {
                if (invalidate) delay(NETWORK_DEBOUNCE_MS)
                val endpoint = refresh(targetGeneration, reason)
                val context = applicationContext
                if (
                    endpoint == null &&
                    context != null &&
                    isCurrent(targetGeneration) &&
                    isEligibleWifi(context) &&
                    ContextSyncPreferences(context).serverId != null
                ) {
                    // Keep NSD nearly continuous while the trusted service is absent.
                    scope.launch {
                        delay(NETWORK_DEBOUNCE_MS)
                        triggerRefresh("discovery_retry", invalidate = false)
                    }
                }
            }
        }
    }

    private suspend fun refresh(targetGeneration: Long, reason: String): ContextSyncEndpoint? =
        refreshMutex.withLock {
            val context = applicationContext ?: return@withLock null
            if (!isCurrent(targetGeneration) || !isEligibleWifi(context)) {
                trustedPresence = null
                consecutiveConnectFailures = 0
                consecutiveForwardSuccesses = 0
                ReverseLanManager.stopAndAwait("wifi_unavailable")
                clearEndpointIfCurrent(context, targetGeneration, "wifi_unavailable")
                return@withLock null
            }

            val preferences = ContextSyncPreferences(context)
            var trustedServerId = preferences.serverId
            val cachedOrigin = preferences.localOrigin
            if (cachedOrigin != null && trustedServerId != null) {
                val cachedProbe = ContextVideoSyncApi.probe(cachedOrigin, trustedServerId)
                if (!isCurrent(targetGeneration)) return@withLock null
                if (cachedProbe is ForwardProbeResult.Verified) {
                    trustedPresence = null
                    return@withLock acceptVerifiedEndpoint(
                        context,
                        preferences,
                        ContextSyncEndpoint(cachedOrigin, trustedServerId),
                        targetGeneration,
                        reason,
                    )
                }
            }
            val presence = try {
                ContextSyncDiscovery(context).discoverPresence()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Throwable) {
                AppLog.w(TAG, "discovery failed reason=$reason generation=$targetGeneration", error)
                null
            }
            if (!isCurrent(targetGeneration)) return@withLock null
            trustedServerId = preferences.serverId
            trustedPresence = presence
            if (trustedServerId == null) {
                consecutiveConnectFailures = 0
                consecutiveForwardSuccesses = 0
                ReverseLanManager.stopAndAwait("trusted_identity_unavailable")
                return@withLock null
            }

            val endpoints = buildList {
                presence?.endpoints()?.let(::addAll)
            }.distinctBy(ContextSyncEndpoint::origin)
            val probeResults = mutableListOf<ForwardProbeResult>()
            var endpoint: ContextSyncEndpoint? = null
            for (candidate in endpoints) {
                val probe = ContextVideoSyncApi.probe(candidate.origin, trustedServerId)
                if (!isCurrent(targetGeneration)) return@withLock null
                probeResults += probe
                if (probe is ForwardProbeResult.Verified) {
                    endpoint = candidate
                    break
                }
            }
            if (endpoint == null) {
                if (cachedOrigin != null && preferences.clearEndpoint(cachedOrigin)) {
                    logEndpointCleared(cachedOrigin, "health_check_failed", targetGeneration)
                }
                consecutiveForwardSuccesses = 0
                val previousFailures = consecutiveConnectFailures
                consecutiveConnectFailures = nextConnectFailureCount(
                    previousFailures,
                    presence != null,
                    probeResults,
                )
                val connectFailure = consecutiveConnectFailures > previousFailures
                AppLog.event(
                    TAG,
                    "forward_probe_failed",
                    JSONObject()
                        .put("presence", presence != null)
                        .put("connectFailures", consecutiveConnectFailures)
                        .put("generation", targetGeneration),
                    immediateUpload = true,
                )
                when {
                    presence == null ->
                        ReverseLanManager.stopAndAwait("trusted_presence_absent")
                    consecutiveConnectFailures >= REVERSE_ACTIVATION_FAILURES ->
                        ReverseLanManager.activate(trustedServerId)
                    !connectFailure ->
                        ReverseLanManager.stopAndAwait("forward_probe_rejected")
                }
                return@withLock null
            }

            acceptVerifiedEndpoint(
                context,
                preferences,
                endpoint,
                targetGeneration,
                reason,
            )
        }

    private suspend fun acceptVerifiedEndpoint(
        context: Context,
        preferences: ContextSyncPreferences,
        endpoint: ContextSyncEndpoint,
        targetGeneration: Long,
        reason: String,
    ): ContextSyncEndpoint {
        consecutiveConnectFailures = 0
        consecutiveForwardSuccesses += 1
        val changed = preferences.saveEndpoint(endpoint.origin, endpoint.serverId)
        if (changed) {
            AppLog.event(
                TAG,
                "endpoint_verified",
                JSONObject()
                    .put("origin", endpoint.origin)
                    .put("serverIdPrefix", endpoint.serverId.take(8))
                    .put("generation", targetGeneration)
                    .put("reason", reason),
                immediateUpload = true,
            )
            enqueueDependentWork(context)
        }
        if (consecutiveForwardSuccesses >= FORWARD_RECOVERY_SUCCESSES) {
            ReverseLanManager.deactivateAfterDrain("forward_recovered")
        }
        return endpoint
    }

    private fun clearEndpointIfCurrent(context: Context, targetGeneration: Long, reason: String) {
        if (!isCurrent(targetGeneration)) return
        val preferences = ContextSyncPreferences(context)
        val origin = preferences.localOrigin ?: return
        if (preferences.clearEndpoint(origin)) {
            logEndpointCleared(origin, reason, targetGeneration)
        }
    }

    private fun logEndpointCleared(origin: String, reason: String, targetGeneration: Long) {
        AppLog.event(
            TAG,
            "endpoint_cleared",
            JSONObject()
                .put("origin", origin)
                .put("reason", reason)
                .put("generation", targetGeneration),
            immediateUpload = true,
        )
    }

    private fun enqueueDependentWork(context: Context) {
        ContextVideoSyncScheduler.enqueue(context)
        AudioSyncScheduler.enqueue(context)
        if (AppModeRepository.current() == AppMode.NORMAL) {
            StandaloneSyncScheduler.enqueue(context)
        }
    }

    private fun isCurrent(targetGeneration: Long): Boolean =
        isGenerationCurrent(generation.get(), targetGeneration, paused, NetworkAccessPolicy.isAllowed())

    internal fun isEligibleWifi(context: Context): Boolean {
        val connectivity = context.getSystemService(ConnectivityManager::class.java)
        val network = connectivity.activeNetwork ?: return false
        val capabilities = connectivity.getNetworkCapabilities(network) ?: return false
        return capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) &&
            !capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)
    }

    fun hasLocalNetworkPermission(context: Context): Boolean =
        Build.VERSION.SDK_INT < 37 ||
            ContextCompat.checkSelfPermission(
                context,
                Manifest.permission.ACCESS_LOCAL_NETWORK,
            ) == PackageManager.PERMISSION_GRANTED

    internal fun isGenerationCurrent(
        currentGeneration: Long,
        candidateGeneration: Long,
        paused: Boolean,
        networkAllowed: Boolean,
    ): Boolean = currentGeneration == candidateGeneration && !paused && networkAllowed

    internal fun shouldAcceptEndpoint(
        currentGeneration: Long,
        candidateGeneration: Long,
        paused: Boolean,
        networkAllowed: Boolean,
        trustedServerId: String?,
        candidate: ContextSyncEndpoint,
    ): Boolean =
        isGenerationCurrent(currentGeneration, candidateGeneration, paused, networkAllowed) &&
            trustedServerId != null &&
            ContextSyncDiscovery.serverIdsMatch(trustedServerId, candidate.serverId)

    internal fun nextConnectFailureCount(
        previous: Int,
        presenceAvailable: Boolean,
        results: List<ForwardProbeResult>,
    ): Int =
        if (
            presenceAvailable &&
            results.isNotEmpty() &&
            results.all { it is ForwardProbeResult.ConnectFailed }
        ) {
            previous + 1
        } else {
            0
        }

    private const val REVERSE_ACTIVATION_FAILURES = 2
    private const val FORWARD_RECOVERY_SUCCESSES = 2
}
