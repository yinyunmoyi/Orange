package com.example.orange.data.standalone

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import java.util.concurrent.TimeUnit

object StandaloneSyncScheduler {
    fun start(context: Context) {
        StandaloneRepository.init(context)
        if (!StandaloneRepository.isEnabled()) enqueue(context)
    }

    fun enqueue(context: Context) {
        val request = OneTimeWorkRequestBuilder<StandaloneSyncWorker>()
            .setConstraints(
                Constraints.Builder()
                    .setRequiredNetworkType(NetworkType.CONNECTED)
                    .build(),
            )
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS)
            .build()
        WorkManager.getInstance(context.applicationContext).enqueueUniqueWork(
            UNIQUE_WORK,
            ExistingWorkPolicy.KEEP,
            request,
        )
    }

    internal const val UNIQUE_WORK = "standalone-favorites-sync"
}
