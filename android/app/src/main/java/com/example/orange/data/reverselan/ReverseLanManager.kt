package com.example.orange.data.reverselan

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.SystemClock
import android.util.Base64
import com.example.orange.BuildConfig
import com.example.orange.data.logging.AppLog
import java.security.SecureRandom
import java.util.UUID
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import org.json.JSONObject

object ReverseLanManager : ReverseLanTransport {
    private const val TAG = "ReverseLan"
    private const val SERVICE_TYPE = "_orange-client._tcp."
    private const val CLIENT_PREFERENCES = "reverse_lan"
    private const val CLIENT_ID_KEY = "client_id"
    private const val BRIDGE_STALE_MILLIS = 35_000L
    private const val DRAIN_TIMEOUT_MILLIS = 10_000L

    private data class ActiveRelay(
        val context: Context,
        val serverId: String,
        val generation: String,
        val queue: ReverseLanQueue,
        val server: ReverseLanServer,
        val registrationListener: NsdManager.RegistrationListener,
        val lastPollAt: AtomicLong,
        var watchdog: Job?,
    )

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val lifecycleMutex = Mutex()
    private val mutableState = MutableStateFlow(ReverseLanState.STOPPED)

    @Volatile
    private var applicationContext: Context? = null

    @Volatile
    private var active: ActiveRelay? = null

    override val state: StateFlow<ReverseLanState> = mutableState.asStateFlow()

    fun initialize(context: Context) {
        applicationContext = context.applicationContext
    }

    suspend fun activate(serverId: String) {
        if (!BuildConfig.REVERSE_LAN_ENABLED) return
        require(SERVER_ID_PATTERN.matches(serverId)) { "invalid reverse LAN server id" }
        val context = applicationContext ?: return
        lifecycleMutex.withLock {
            if (active?.serverId == serverId) return
            stopLocked("replace")
            mutableState.value = ReverseLanState.ACTIVATING
            val generation = UUID.randomUUID().toString()
            val secretBytes = ByteArray(32).also(SecureRandom()::nextBytes)
            val secret = Base64.encodeToString(
                secretBytes,
                Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP,
            )
            val queue = ReverseLanQueue()
            val lastPollAt = AtomicLong(0L)
            val server = ReverseLanServer(serverId, generation, secret, queue) {
                lastPollAt.set(SystemClock.elapsedRealtime())
                val current = active
                if (current != null && current.generation == generation) {
                    if (mutableState.value != ReverseLanState.DRAINING) {
                        val firstAuthentication = mutableState.value != ReverseLanState.READY
                        mutableState.value = ReverseLanState.READY
                        if (firstAuthentication) {
                            AppLog.event(
                                TAG,
                                "bridge_authenticated",
                                JSONObject().put("generation", generation),
                                immediateUpload = true,
                            )
                        }
                    }
                }
            }
            try {
                AppLog.event(
                    TAG,
                    "activation_started",
                    JSONObject()
                        .put("serverIdPrefix", serverId.take(8))
                        .put("generation", generation),
                    immediateUpload = true,
                )
                server.start(5_000, false)
                check(server.boundPort in 1..65535) { "reverse LAN server did not bind" }
                withContext(Dispatchers.IO) {
                    ReverseLanRegistrationApi.register(
                        clientId = clientId(context),
                        serverId = serverId,
                        generation = generation,
                        secret = secret,
                    )
                }
                AppLog.event(
                    TAG,
                    "registration_accepted",
                    JSONObject().put("generation", generation),
                    immediateUpload = true,
                )
                val listener = registerService(
                    context = context,
                    clientId = clientId(context),
                    serverId = serverId,
                    generation = generation,
                    port = server.boundPort,
                )
                val relay = ActiveRelay(
                    context = context,
                    serverId = serverId,
                    generation = generation,
                    queue = queue,
                    server = server,
                    registrationListener = listener,
                    lastPollAt = lastPollAt,
                    watchdog = null,
                )
                active = relay
                mutableState.value = if (lastPollAt.get() > 0L) {
                    ReverseLanState.READY
                } else {
                    ReverseLanState.WAITING_FOR_BRIDGE
                }
                if (mutableState.value == ReverseLanState.READY) {
                    AppLog.event(
                        TAG,
                        "bridge_authenticated",
                        JSONObject().put("generation", generation),
                        immediateUpload = true,
                    )
                }
                relay.watchdog = startWatchdog(relay)
                AppLog.event(
                    TAG,
                    "relay_advertised",
                    JSONObject()
                        .put("serverIdPrefix", serverId.take(8))
                        .put("generation", generation)
                        .put("port", server.boundPort),
                    immediateUpload = true,
                )
            } catch (error: Throwable) {
                queue.stop()
                server.stop()
                active = null
                mutableState.value = ReverseLanState.STOPPED
                AppLog.w(TAG, "activation failed", error, immediateUpload = true)
            }
        }
    }

    override suspend fun execute(request: ReverseLanRequest): ReverseLanResult {
        val relay = active ?: return ReverseLanResult.Unavailable
        if (mutableState.value != ReverseLanState.READY) return ReverseLanResult.Unavailable
        return relay.queue.execute(request)
    }

    suspend fun stopAndAwait(reason: String) {
        lifecycleMutex.withLock {
            stopLocked(reason)
        }
    }

    suspend fun deactivateAfterDrain(reason: String) {
        lifecycleMutex.withLock {
            val relay = active ?: return
            mutableState.value = ReverseLanState.DRAINING
            val deadline = SystemClock.elapsedRealtime() + DRAIN_TIMEOUT_MILLIS
            while (
                relay.queue.pendingCount() > 0 &&
                SystemClock.elapsedRealtime() < deadline
            ) {
                delay(50L)
            }
            stopLocked(reason)
        }
    }

    private suspend fun stopLocked(reason: String) {
        val relay = active ?: run {
            mutableState.value = ReverseLanState.STOPPED
            return
        }
        active = null
        relay.watchdog?.cancel()
        relay.queue.stop()
        relay.server.stop()
        unregisterService(relay.context, relay.registrationListener)
        mutableState.value = ReverseLanState.STOPPED
        AppLog.event(
            TAG,
            "relay_stopped",
            JSONObject().put("reason", reason).put("generation", relay.generation),
            immediateUpload = true,
        )
    }

    private fun startWatchdog(relay: ActiveRelay): Job = scope.launch {
        while (isActive && active?.generation == relay.generation) {
            delay(5_000L)
            val lastPollAt = relay.lastPollAt.get()
            if (
                lastPollAt > 0L &&
                SystemClock.elapsedRealtime() - lastPollAt > BRIDGE_STALE_MILLIS &&
                mutableState.value == ReverseLanState.READY
            ) {
                mutableState.value = ReverseLanState.WAITING_FOR_BRIDGE
                AppLog.event(
                    TAG,
                    "bridge_stale",
                    JSONObject().put("generation", relay.generation),
                    immediateUpload = true,
                )
            }
        }
    }

    private suspend fun registerService(
        context: Context,
        clientId: String,
        serverId: String,
        generation: String,
        port: Int,
    ): NsdManager.RegistrationListener {
        val manager = context.getSystemService(NsdManager::class.java)
        val serviceInfo = NsdServiceInfo().apply {
            serviceName = "Orange Client ${clientId.take(8)} ${generation.take(8)}"
            serviceType = SERVICE_TYPE
            this.port = port
            setAttribute("api", REVERSE_LAN_API_VERSION.toString())
            setAttribute("client", clientId)
            setAttribute("server", serverId)
            setAttribute("generation", generation)
        }
        lateinit var listener: NsdManager.RegistrationListener
        val registered = withTimeoutOrNull(3_000L) {
            suspendCancellableCoroutine<NsdManager.RegistrationListener> { continuation ->
                listener = object : NsdManager.RegistrationListener {
                    override fun onServiceRegistered(serviceInfo: NsdServiceInfo) {
                        if (continuation.isActive) continuation.resume(this)
                    }

                    override fun onRegistrationFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                        if (continuation.isActive) {
                            continuation.resumeWithException(
                                IllegalStateException("NSD registration failed: $errorCode"),
                            )
                        }
                    }

                    override fun onServiceUnregistered(serviceInfo: NsdServiceInfo) = Unit
                    override fun onUnregistrationFailed(serviceInfo: NsdServiceInfo, errorCode: Int) = Unit
                }
                continuation.invokeOnCancellation {
                    runCatching { manager.unregisterService(listener) }
                }
                manager.registerService(serviceInfo, NsdManager.PROTOCOL_DNS_SD, listener)
            }
        }
        return registered ?: throw IllegalStateException("NSD registration timed out")
    }

    private suspend fun unregisterService(
        context: Context,
        listener: NsdManager.RegistrationListener,
    ) {
        val manager = context.getSystemService(NsdManager::class.java)
        withContext(Dispatchers.IO) {
            runCatching { manager.unregisterService(listener) }
        }
    }

    private fun clientId(context: Context): String {
        val preferences = context.getSharedPreferences(CLIENT_PREFERENCES, Context.MODE_PRIVATE)
        preferences.getString(CLIENT_ID_KEY, null)?.let { existing ->
            if (runCatching { UUID.fromString(existing) }.isSuccess) return existing
        }
        val generated = UUID.randomUUID().toString()
        check(preferences.edit().putString(CLIENT_ID_KEY, generated).commit()) {
            "failed to persist reverse LAN client id"
        }
        return generated
    }

    private val SERVER_ID_PATTERN = Regex("^[a-f0-9]{32}$")
}
