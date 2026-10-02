package com.example.orange.data.mode

import android.content.Context
import androidx.work.WorkManager
import com.example.orange.data.audiosync.AudioSyncScheduler
import com.example.orange.data.contextsync.ContextVideoSyncScheduler
import com.example.orange.data.logging.AppLog
import com.example.orange.data.network.NetworkAccessPolicy
import com.example.orange.data.offline.OfflineCapabilityRepository
import com.example.orange.data.offline.OfflineEngineSession
import com.example.orange.data.standalone.StandaloneRepository
import com.example.orange.data.standalone.StandaloneSyncScheduler
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

object AppModeController {
    private val mutex = Mutex()

    suspend fun switchTo(context: Context, target: AppMode) = mutex.withLock {
        val appContext = context.applicationContext
        if (target == AppMode.OFFLINE) {
            check(OfflineCapabilityRepository.verifiedResult() != null) {
                "Offline capability has not been verified"
            }
            val previousMode = AppModeRepository.current()
            NetworkAccessPolicy.setOffline(true)
            cancelNetworkWork(appContext)
            OfflineEngineSession.warmUp().getOrElse { error ->
                OfflineEngineSession.close()
                NetworkAccessPolicy.setOffline(false)
                resumeNetworkWork(appContext, previousMode)
                throw error
            }
            StandaloneRepository.setLegacyEnabled(false)
            AppModeRepository.set(AppMode.OFFLINE)
            OfflineCapabilityRepository.enableVerified()
            return@withLock
        }

        OfflineEngineSession.close()
        StandaloneRepository.setLegacyEnabled(target == AppMode.STANDALONE)
        OfflineCapabilityRepository.disable()
        NetworkAccessPolicy.setOffline(false)
        AppModeRepository.set(target)
        resumeNetworkWork(appContext, target)
    }

    private fun resumeNetworkWork(context: Context, mode: AppMode) {
        ContextVideoSyncScheduler.start(context)
        AudioSyncScheduler.start(context)
        if (mode == AppMode.NORMAL) StandaloneSyncScheduler.enqueue(context)
        AppLog.resumeNetwork()
    }

    fun initialize(context: Context) {
        AppModeRepository.init(context)
        NetworkAccessPolicy.initialize(AppModeRepository.current())
    }

    private fun cancelNetworkWork(context: Context) {
        WorkManager.getInstance(context).apply {
            cancelUniqueWork(StandaloneSyncScheduler.UNIQUE_WORK)
            cancelUniqueWork(AudioSyncScheduler.UNIQUE_WORK)
            cancelUniqueWork(ContextVideoSyncScheduler.UNIQUE_WORK)
        }
    }
}
