package com.example.orange

import android.app.Activity
import android.app.Application
import android.os.Bundle
import com.example.orange.data.audiosync.AudioSyncScheduler
import com.example.orange.data.contextsync.ContextVideoSyncScheduler
import com.example.orange.data.logging.AppLog
import com.example.orange.data.word.WordServiceConfig

class OrangeApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        WordServiceConfig.init(this)
        AppLog.init(this)
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
        ContextVideoSyncScheduler.start(this)
        AudioSyncScheduler.start(this)
    }

    override fun onTrimMemory(level: Int) {
        AppLog.w("Application", "memory trim requested level=$level")
        super.onTrimMemory(level)
    }
}
