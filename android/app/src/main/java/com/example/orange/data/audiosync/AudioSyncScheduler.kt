package com.example.orange.data.audiosync

import android.content.Context
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import com.example.orange.data.contextsync.ContextSyncConnectionManager
import com.example.orange.data.network.NetworkAccessPolicy

object AudioSyncScheduler {
    fun start(context: Context) {
        if (!NetworkAccessPolicy.isAllowed()) return
        val appContext = context.applicationContext
        if (!ContextSyncConnectionManager.hasLocalNetworkPermission(appContext)) return
        enqueue(appContext)
    }

    fun enqueue(context: Context) {
        if (!NetworkAccessPolicy.isAllowed()) return
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
