package com.example.orange.data.mode

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import com.example.orange.data.logging.AppLog
import com.example.orange.data.network.InternetAvailability
import com.example.orange.data.offline.OfflineCapabilityRepository
import com.example.orange.data.offline.OfflineEngineSession
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

enum class OfflineAutoModeState {
    IDLE,
    INITIALIZING,
    READY,
    DISABLING,
    FAILED,
}

internal enum class OfflineNetworkAction {
    STAY_ONLINE,
    ENABLE_OFFLINE,
    STAY_OFFLINE,
    DISABLE_OFFLINE,
}

object OfflineAutoModeController {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val mutex = Mutex()
    private val mutableState = MutableStateFlow(OfflineAutoModeState.IDLE)
    private var started = false
    private var scheduledCheck: Job? = null

    val state: StateFlow<OfflineAutoModeState> = mutableState.asStateFlow()

    fun start(context: Context) {
        val appContext = context.applicationContext
        synchronized(this) {
            if (started) return
            started = true
            val manager = appContext.getSystemService(ConnectivityManager::class.java)
            if (manager != null) {
                manager.registerDefaultNetworkCallback(
                    object : ConnectivityManager.NetworkCallback() {
                        override fun onAvailable(network: Network) = scheduleCheck(appContext)

                        override fun onLost(network: Network) = scheduleCheck(appContext)

                        override fun onCapabilitiesChanged(
                            network: Network,
                            capabilities: NetworkCapabilities,
                        ) = scheduleCheck(appContext)
                    },
                )
            }
        }
        check(appContext)
    }

    fun check(context: Context) {
        val appContext = context.applicationContext
        scope.launch {
            reconcile(appContext)
        }
    }

    private fun scheduleCheck(context: Context) {
        synchronized(this) {
            scheduledCheck?.cancel()
            scheduledCheck = scope.launch {
                delay(NETWORK_SETTLE_MILLIS)
                reconcile(context)
            }
        }
    }

    private suspend fun reconcile(context: Context) = mutex.withLock {
        val mode = AppModeRepository.current()
        when (offlineNetworkAction(mode, InternetAvailability.isAvailable(context))) {
            OfflineNetworkAction.STAY_ONLINE -> {
                mutableState.value = OfflineAutoModeState.IDLE
                clearPreviousOnlineMode(context)
                return@withLock
            }
            OfflineNetworkAction.DISABLE_OFFLINE -> {
                mutableState.value = OfflineAutoModeState.DISABLING
                val target = previousOnlineMode(context)
                runCatching {
                    AppModeController.switchTo(context, target)
                }.fold(
                    onSuccess = {
                        clearPreviousOnlineMode(context)
                        mutableState.value = OfflineAutoModeState.IDLE
                        AppLog.i("OfflineCapability", "automatic offline mode disabled")
                    },
                    onFailure = { error ->
                        mutableState.value = OfflineAutoModeState.READY
                        AppLog.e(
                            "OfflineCapability",
                            "automatic offline mode disable failed",
                            error,
                        )
                    },
                )
                return@withLock
            }
            OfflineNetworkAction.STAY_OFFLINE -> {
                if (mutableState.value == OfflineAutoModeState.READY) return@withLock
            }
            OfflineNetworkAction.ENABLE_OFFLINE -> savePreviousOnlineMode(context, mode)
        }

        mutableState.value = OfflineAutoModeState.INITIALIZING
        AppLog.i("OfflineCapability", "automatic offline initialization started")
        val result = if (OfflineCapabilityRepository.verifiedResult() == null) {
            Result.failure(IllegalStateException("本地模型尚未正确配置"))
        } else {
            runCatching {
                if (mode == AppMode.OFFLINE) {
                    OfflineEngineSession.warmUp().getOrThrow()
                } else {
                    AppModeController.switchTo(context, AppMode.OFFLINE)
                }
            }
        }
        result.fold(
            onSuccess = {
                mutableState.value = OfflineAutoModeState.READY
                AppLog.i("OfflineCapability", "automatic offline initialization completed")
            },
            onFailure = { error ->
                mutableState.value = OfflineAutoModeState.FAILED
                AppLog.e("OfflineCapability", "automatic offline initialization failed", error)
            },
        )
    }

    private fun savePreviousOnlineMode(context: Context, mode: AppMode) {
        if (mode == AppMode.OFFLINE) return
        preferences(context).edit().putString(KEY_PREVIOUS_MODE, mode.name).apply()
    }

    private fun previousOnlineMode(context: Context): AppMode =
        resolvePreviousOnlineMode(preferences(context).getString(KEY_PREVIOUS_MODE, null))

    private fun clearPreviousOnlineMode(context: Context) {
        preferences(context).edit().remove(KEY_PREVIOUS_MODE).apply()
    }

    private fun preferences(context: Context) =
        context.getSharedPreferences(PREFERENCES_FILE, Context.MODE_PRIVATE)

    private const val NETWORK_SETTLE_MILLIS = 1_000L
    private const val PREFERENCES_FILE = "offline_auto_mode"
    private const val KEY_PREVIOUS_MODE = "previous_online_mode"
}

internal fun offlineNetworkAction(
    mode: AppMode,
    internetAvailable: Boolean,
): OfflineNetworkAction = when {
    internetAvailable && mode == AppMode.OFFLINE -> OfflineNetworkAction.DISABLE_OFFLINE
    internetAvailable -> OfflineNetworkAction.STAY_ONLINE
    mode == AppMode.OFFLINE -> OfflineNetworkAction.STAY_OFFLINE
    else -> OfflineNetworkAction.ENABLE_OFFLINE
}

internal fun resolvePreviousOnlineMode(savedMode: String?): AppMode =
    savedMode
        ?.let { runCatching { AppMode.valueOf(it) }.getOrNull() }
        ?.takeIf { it != AppMode.OFFLINE }
        ?: AppMode.NORMAL
