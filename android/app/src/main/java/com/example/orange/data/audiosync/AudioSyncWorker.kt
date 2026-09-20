package com.example.orange.data.audiosync

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import com.example.orange.data.logging.AppLog as Log
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.example.orange.data.contextsync.ContextSyncDiscovery
import com.example.orange.data.contextsync.ContextVideoSyncWorker

class AudioSyncWorker(
    appContext: Context,
    workerParams: WorkerParameters,
) : CoroutineWorker(appContext, workerParams) {
    override suspend fun doWork(): Result {
        if (!ContextVideoSyncWorker.hasLocalNetworkPermission(applicationContext)) {
            return Result.success()
        }
        val connectivity = applicationContext.getSystemService(ConnectivityManager::class.java)
        val network = connectivity.activeNetwork ?: return Result.success()
        val capabilities = connectivity.getNetworkCapabilities(network) ?: return Result.success()
        if (
            !capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) ||
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)
        ) {
            return Result.success()
        }

        val endpoint = ContextSyncDiscovery(applicationContext).discover()
            ?: return Result.success()
        val manifest = AudioSyncApi.manifest(endpoint.origin, endpoint.serverId)
            .getOrElse {
                Log.w(TAG, "manifest failed origin=${endpoint.origin}", it)
                return if (runAttemptCount < 2) Result.retry() else Result.success()
            }
        val summary = AudioCacheRepository.sync(applicationContext, endpoint, manifest)
        Log.i(
            TAG,
            "sync complete items=${manifest.items.size} hits=${summary.hits} " +
                "downloaded=${summary.downloaded} failed=${summary.failed} " +
                "deleted=${summary.deleted} bytes=${summary.downloadedBytes}",
        )
        return Result.success()
    }

    companion object {
        private const val TAG = "WordPhraseAudioSync"
    }
}
