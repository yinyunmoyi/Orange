package com.example.orange.data.standalone

import android.content.Context
import com.example.orange.data.contextsync.ContextSyncDiscovery
import com.example.orange.data.logging.AppLog as Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters

class StandaloneSyncWorker(
    appContext: Context,
    workerParams: WorkerParameters,
) : CoroutineWorker(appContext, workerParams) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        StandaloneRepository.init(applicationContext)
        StandaloneRepository.setSyncInProgress(true)
        try {
            val store = StandaloneRepository.store()
            val localOrigin = runCatching {
                ContextSyncDiscovery(applicationContext).discover()?.origin
            }.getOrNull()
            val origins = StandaloneSyncApi.candidateOrigins(localOrigin)
            var hadFailure = false

            for (initial in store.listFavorites()) {
                var favorite = initial
                if (favorite.syncState == StandaloneSyncState.PENDING) {
                    val mapping = tryOrigins(origins) {
                        StandaloneSyncApi.syncFavorite(it, favorite)
                    }
                    if (mapping == null) {
                        store.markFavoriteFailed(favorite.clientId, "favorite sync failed")
                        hadFailure = true
                        continue
                    }
                    store.markFavoriteMapped(favorite.clientId, mapping.itemId)
                    StandaloneRepository.refresh()
                    favorite = favorite.copy(
                        syncState = StandaloneSyncState.MAPPED,
                        serverId = mapping.itemId,
                    )
                }

                val note = store.getNote(favorite.itemType, favorite.text)
                if (note != null && note.revision > note.syncedRevision) {
                    val synced = tryOrigins(origins) {
                        StandaloneSyncApi.syncNote(it, favorite, note)
                    } != null
                    if (synced) {
                        store.confirmNote(favorite.itemType, favorite.text, note.revision)
                    } else {
                        hadFailure = true
                    }
                }

                for (context in store.pendingContexts(favorite.clientId)) {
                    val synced = tryOrigins(origins) {
                        StandaloneSyncApi.syncContext(it, favorite, context)
                    } != null
                    if (synced) store.confirmContext(context.eventId) else hadFailure = true
                }

                for (action in store.pendingActions(favorite.clientId)) {
                    val synced = tryOrigins(origins) {
                        StandaloneSyncApi.syncAction(it, favorite, action)
                    } != null
                    if (synced) store.confirmAction(action.eventId) else hadFailure = true
                }
                store.cleanupMappedFavorite(favorite.clientId)
            }

            StandaloneRepository.refresh()
            val preferences = StandaloneRepository.syncPreferences()
            if (hadFailure) {
                preferences.lastSyncError = "部分数据同步失败"
                Log.w(TAG, "standalone sync incomplete attempt=$runAttemptCount")
                Result.retry()
            } else {
                preferences.lastSyncError = null
                preferences.lastSyncAt = System.currentTimeMillis()
                Result.success()
            }
        } catch (error: Throwable) {
            StandaloneRepository.syncPreferences().lastSyncError =
                error.message?.take(200) ?: error.javaClass.simpleName
            Log.e(TAG, "standalone sync crashed attempt=$runAttemptCount", error)
            Result.retry()
        } finally {
            StandaloneRepository.setSyncInProgress(false)
        }
    }

    private fun <T> tryOrigins(origins: List<String>, request: (String) -> T): T? {
        for (origin in origins) {
            try {
                return request(origin)
            } catch (error: Throwable) {
                Log.w(TAG, "request failed origin=$origin type=${error.javaClass.simpleName}")
            }
        }
        return null
    }

    private companion object {
        const val TAG = "StandaloneSync"
    }
}
