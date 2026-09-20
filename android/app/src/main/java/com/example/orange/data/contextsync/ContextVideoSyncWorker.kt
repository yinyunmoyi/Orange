package com.example.orange.data.contextsync

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Build
import com.example.orange.data.logging.AppLog as Log
import androidx.core.content.ContextCompat
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters

class ContextVideoSyncWorker(
    appContext: Context,
    workerParams: WorkerParameters,
) : CoroutineWorker(appContext, workerParams) {
    override suspend fun doWork(): Result {
        if (!hasLocalNetworkPermission(applicationContext)) return Result.success()
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
        val manifest = ContextVideoSyncApi.manifest(endpoint.origin, endpoint.serverId)
            .getOrElse {
                Log.w(TAG, "manifest failed origin=${endpoint.origin}", it)
                return if (runAttemptCount < 2) Result.retry() else Result.success()
            }
        val summary = ContextVideoCacheRepository.sync(
            applicationContext,
            endpoint,
            manifest,
        )
        Log.i(
            TAG,
            "sync complete items=${manifest.items.size} hits=${summary.hits} " +
                "migrated=${summary.migrated} downloaded=${summary.downloaded} " +
                "failed=${summary.failed} evicted=${summary.evicted} bytes=${summary.downloadedBytes}",
        )
        return Result.success()
    }

    companion object {
        private const val TAG = "ContextVideoSync"

        fun hasLocalNetworkPermission(context: Context): Boolean =
            Build.VERSION.SDK_INT < 37 ||
                ContextCompat.checkSelfPermission(
                    context,
                    Manifest.permission.ACCESS_LOCAL_NETWORK,
                ) == PackageManager.PERMISSION_GRANTED
    }
}
