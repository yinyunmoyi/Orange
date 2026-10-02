package com.example.orange.data.offline

import com.example.orange.data.logging.AppLog
import java.util.UUID
import org.json.JSONObject

internal object OfflineDiagnostics {
    private const val TAG = "OfflineCapability"

    fun newAttemptId(): String = UUID.randomUUID().toString()

    fun event(
        message: String,
        traceId: String?,
        data: JSONObject = JSONObject(),
    ) {
        AppLog.event(TAG, message, data, traceId)
    }

    fun failure(
        message: String,
        traceId: String?,
        error: Throwable,
        data: JSONObject = JSONObject(),
    ) {
        data.put("errorType", error.javaClass.name)
        error.message
            ?.takeIf(String::isNotBlank)
            ?.let { data.put("errorSummary", sanitizeOfflineErrorMessage(it)) }
        if (error is OfflineValidationException) {
            data.put("errorCode", error.error.name)
        }
        AppLog.event(TAG, message, data, traceId)
    }
}

private val URI_PATTERN = Regex("""\b[a-zA-Z][a-zA-Z0-9+.-]*://\S+""")
private val ABSOLUTE_PATH_PATTERN = Regex("""(?<![A-Za-z0-9_])/(?:[^\s/,;:，；：）\]}]+/)*[^\s,;:，；：）\]}]+""")

internal fun sanitizeOfflineErrorMessage(message: String): String =
    message
        .replace(URI_PATTERN, "<uri>")
        .replace(ABSOLUTE_PATH_PATTERN, "<path>")
        .take(500)
