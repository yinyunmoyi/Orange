package com.example.orange.data.audiosync

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import com.example.orange.data.contextsync.ContextVideoSyncWorker
import java.util.concurrent.atomic.AtomicBoolean

object AudioSyncScheduler {
    private val callbackRegistered = AtomicBoolean(false)

    fun start(context: Context) {
        val appContext = context.applicationContext
        if (!ContextVideoSyncWorker.hasLocalNetworkPermission(appContext)) return
        enqueue(appContext)
        if (!callbackRegistered.compareAndSet(false, true)) return
        val connectivity = appContext.getSystemService(ConnectivityManager::class.java)
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            .build()
        connectivity.registerNetworkCallback(
            request,
            object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    enqueue(appContext)
                }
            },
        )
    }

    fun enqueue(context: Context) {
        val constraints = Constraints.Builder()
            .setRequiredNetworkType(NetworkType.UNMETERED)
            .setRequiresStorageNotLow(true)
            .build()
        val request = OneTimeWorkRequestBuilder<AudioSyncWorker>()
            .setConstraints(constraints)
            .build()
        WorkManager.getInstance(context.applicationContext).enqueueUniqueWork(
            UNIQUE_WORK,
            ExistingWorkPolicy.KEEP,
            request,
        )
    }

    internal const val UNIQUE_WORK = "word-phrase-audio-lan-sync"
}
