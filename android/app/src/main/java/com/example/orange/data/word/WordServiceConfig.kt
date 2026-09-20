package com.example.orange.data.word

import android.content.Context
import com.example.orange.BuildConfig
import java.net.URI

internal data class WordServiceEndpoint(
    val scheme: String,
    val host: String,
    val port: Int,
) {
    val origin: String
        get() {
            val defaultPort = if (scheme == "https") 443 else 80
            return if (port == defaultPort) "$scheme://$host" else "$scheme://$host:$port"
        }
}

internal fun parseWordServiceEndpoint(raw: String): WordServiceEndpoint {
    val normalized = raw.trim().trimEnd('/')
    val uri = runCatching { URI(normalized) }
        .getOrElse { throw IllegalArgumentException("Invalid ANDROID_PUBLIC_BASE_URL", it) }
    val scheme = uri.scheme?.lowercase()
    require(scheme == "http" || scheme == "https") {
        "ANDROID_PUBLIC_BASE_URL must use http or https"
    }
    require(!uri.host.isNullOrBlank() && uri.userInfo == null) {
        "ANDROID_PUBLIC_BASE_URL must contain a valid host"
    }
    require(uri.path.isNullOrEmpty() && uri.query == null && uri.fragment == null) {
        "ANDROID_PUBLIC_BASE_URL must be an origin without a path, query, or fragment"
    }
    require(uri.port != 0) { "ANDROID_PUBLIC_BASE_URL contains an invalid port" }
    val port = when {
        uri.port > 0 -> uri.port
        scheme == "https" -> 443
        else -> 80
    }
    require(port in 1..65535) { "ANDROID_PUBLIC_BASE_URL contains an invalid port" }
    return WordServiceEndpoint(scheme, uri.host, port)
}

object WordServiceConfig {
    private const val LOCAL_ENDPOINT_PREFERENCES = "context_video_sync"
    private const val LOCAL_ORIGIN_KEY = "local_origin"
    private const val API_PREFIX = "/api/v1/word"
    private val endpoint = parseWordServiceEndpoint(BuildConfig.ORANGE_PUBLIC_BASE_URL)

    @Volatile
    private var applicationContext: Context? = null

    val scheme: String = endpoint.scheme
    val host: String = endpoint.host
    val port: Int = endpoint.port
    val apiPrefix: String = API_PREFIX

    val origin: String
        get() = endpoint.origin

    val apiBaseUrl: String
        get() = origin + apiPrefix

    fun preferredApiBaseUrls(): List<String> {
        val localOrigin = applicationContext
            ?.getSharedPreferences(LOCAL_ENDPOINT_PREFERENCES, Context.MODE_PRIVATE)
            ?.getString(LOCAL_ORIGIN_KEY, null)
            ?.trim()
            ?.trimEnd('/')
            ?.takeIf { it.startsWith("http://") || it.startsWith("https://") }
        return listOfNotNull(localOrigin?.plus(apiPrefix), apiBaseUrl).distinct()
    }

    fun init(context: Context) {
        applicationContext = context.applicationContext
    }
}
