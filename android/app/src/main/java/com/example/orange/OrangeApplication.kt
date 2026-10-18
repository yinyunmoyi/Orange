package com.example.orange

import android.app.Activity
import android.app.Application
import android.content.ComponentCallbacks2
import android.os.Bundle
import com.example.orange.data.audiosync.AudioSyncScheduler
import com.example.orange.data.contextsync.ContextSyncConnectionManager
import com.example.orange.data.contextsync.ContextVideoSyncScheduler
import com.example.orange.data.logging.AppLog
import com.example.orange.data.mode.AppMode
import com.example.orange.data.mode.AppModeController
import com.example.orange.data.mode.AppModeRepository
import com.example.orange.data.mode.OfflineAutoModeController
import com.example.orange.data.offline.OfflineCapabilityRepository
import com.example.orange.data.offline.OfflineEngineSession
import com.example.orange.data.standalone.StandaloneRepository
import com.example.orange.data.standalone.StandaloneSyncScheduler
import com.example.orange.data.word.WordServiceConfig

class OrangeApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        AppModeController.initialize(this)
        WordServiceConfig.init(this)
        AppLog.init(this)
        StandaloneRepository.init(this)
        OfflineCapabilityRepository.init(this)
        OfflineEngineSession.init(this)
        OfflineAutoModeController.start(this)
        registerActivityLifecycleCallbacks(
            object : ActivityLifecycleCallbacks {
                override fun onActivityCreated(activity: Activity, state: Bundle?) {
                    AppLog.i("AppLifecycle", "${activity.javaClass.simpleName} created")
                }

                override fun onActivityStarted(activity: Activity) {
                    AppLog.i("AppLifecycle", "${activity.javaClass.simpleName} started")
                }

                override fun onActivityResumed(activity: Activity) {
                    AppLog.i("AppLifecycle", "${activity.javaClass.simpleName} resumed")
                }

                override fun onActivityPaused(activity: Activity) {
                    AppLog.i("AppLifecycle", "${activity.javaClass.simpleName} paused")
                }

                override fun onActivityStopped(activity: Activity) {
                    AppLog.i("AppLifecycle", "${activity.javaClass.simpleName} stopped")
                }

                override fun onActivityDestroyed(activity: Activity) {
                    AppLog.i("AppLifecycle", "${activity.javaClass.simpleName} destroyed")
                }

                override fun onActivitySaveInstanceState(activity: Activity, state: Bundle) = Unit
            },
        )
        if (AppModeRepository.current() != AppMode.OFFLINE) {
            ContextSyncConnectionManager.start(this)
            ContextVideoSyncScheduler.start(this)
            AudioSyncScheduler.start(this)
            StandaloneSyncScheduler.start(this)
        }
    }

    override fun onTrimMemory(level: Int) {
        AppLog.w("Application", "memory trim requested level=$level")
        if (
            level == ComponentCallbacks2.TRIM_MEMORY_RUNNING_CRITICAL ||
            level >= ComponentCallbacks2.TRIM_MEMORY_COMPLETE
        ) {
            OfflineEngineSession.closeAsync()
        }
        super.onTrimMemory(level)
    }
}
