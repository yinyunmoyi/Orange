package com.example.orange.data.reverselan

import java.net.URI
import java.security.MessageDigest
import java.util.UUID
import org.json.JSONObject

internal const val REVERSE_LAN_API_VERSION = 1
internal const val REVERSE_LAN_CAPABILITY = "http-json-v1"
internal const val REVERSE_LAN_INTERACTIVE_LANE = "interactive"
internal const val REVERSE_LAN_MAX_COMMAND_BODY_BYTES = 1 shl 20
internal const val REVERSE_LAN_MAX_RESULT_BODY_BYTES = 8 shl 20
internal const val REVERSE_LAN_SERVER_HEADER = "X-Orange-Server-Id"
internal const val REVERSE_LAN_GENERATION_HEADER = "X-Orange-Generation"

data class ReverseLanRequest(
    val apiVersion: Int = REVERSE_LAN_API_VERSION,
    val requestId: String = UUID.randomUUID().toString(),
    val lane: String,
    val method: String,
    val path: String,
    val headers: Map<String, String> = emptyMap(),
    val body: String = "",
) {
    fun validate() {
        require(apiVersion == REVERSE_LAN_API_VERSION) { "unsupported reverse LAN api version" }
        require(runCatching { UUID.fromString(requestId) }.isSuccess) { "invalid request id" }
        require(lane == REVERSE_LAN_INTERACTIVE_LANE || lane == "background") {
            "invalid reverse LAN lane"
        }
        require(method in ALLOWED_METHODS) { "invalid reverse LAN method" }
        require(isRelativeApiPath(path)) { "invalid reverse LAN path" }
        require(body.toByteArray(Charsets.UTF_8).size <= REVERSE_LAN_MAX_COMMAND_BODY_BYTES) {
            "reverse LAN request body too large"
        }
        headers.keys.forEach { name ->
            require(name.lowercase() in ALLOWED_HEADERS) { "invalid reverse LAN header" }
        }
    }

    fun toJson(): JSONObject = JSONObject()
        .put("apiVersion", apiVersion)
        .put("requestId", requestId)
        .put("lane", lane)
        .put("method", method)
        .put("path", path)
        .put("headers", JSONObject(headers))
        .put("body", body)

    companion object {
        private val ALLOWED_METHODS = setOf("GET", "POST", "PUT", "PATCH", "DELETE")
        private val ALLOWED_HEADERS = setOf(
            "accept",
            "content-type",
            "x-orange-debug-trace-id",
        )

        fun fromJson(json: JSONObject): ReverseLanRequest {
            requireOnlyKeys(
                json,
                setOf("apiVersion", "requestId", "lane", "method", "path", "headers", "body"),
            )
            val headerObject = json.optJSONObject("headers") ?: JSONObject()
            val headers = buildMap {
                headerObject.keys().forEach { name ->
                    put(name, headerObject.getString(name))
                }
            }
            return ReverseLanRequest(
                apiVersion = json.getInt("apiVersion"),
                requestId = json.getString("requestId"),
                lane = json.getString("lane"),
                method = json.getString("method"),
                path = json.getString("path"),
                headers = headers,
                body = json.optString("body", ""),
            ).also(ReverseLanRequest::validate)
        }
    }
}

data class ReverseLanResponse(
    val apiVersion: Int = REVERSE_LAN_API_VERSION,
    val requestId: String,
    val status: Int,
    val headers: Map<String, String>,
    val body: String,
) {
    fun validate() {
        require(apiVersion == REVERSE_LAN_API_VERSION) { "unsupported reverse LAN api version" }
        require(runCatching { UUID.fromString(requestId) }.isSuccess) { "invalid request id" }
        require(status in 100..599) { "invalid reverse LAN response status" }
        require(body.toByteArray(Charsets.UTF_8).size <= REVERSE_LAN_MAX_RESULT_BODY_BYTES) {
            "reverse LAN response body too large"
        }
        headers.keys.forEach { name ->
            require(name.equals("Content-Type", ignoreCase = true)) {
                "invalid reverse LAN response header"
            }
        }
    }

    fun stableHash(): String {
        validate()
        val normalized = JSONObject()
            .put("apiVersion", apiVersion)
            .put("requestId", requestId)
            .put("status", status)
            .put("contentType", headers.entries.firstOrNull()?.value.orEmpty())
            .put("body", body)
            .toString()
        return MessageDigest.getInstance("SHA-256")
            .digest(normalized.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
    }

    companion object {
        fun fromJson(json: JSONObject): ReverseLanResponse {
            requireOnlyKeys(
                json,
                setOf("apiVersion", "requestId", "status", "headers", "body"),
            )
            val headerObject = json.optJSONObject("headers") ?: JSONObject()
            val headers = buildMap {
                headerObject.keys().forEach { name ->
                    put(name, headerObject.getString(name))
                }
            }
            return ReverseLanResponse(
                apiVersion = json.getInt("apiVersion"),
                requestId = json.getString("requestId"),
                status = json.getInt("status"),
                headers = headers,
                body = json.optString("body", ""),
            ).also(ReverseLanResponse::validate)
        }
    }
}

sealed interface ReverseLanResult {
    data object Unavailable : ReverseLanResult
    data object NotDispatched : ReverseLanResult
    data object DeliveryUnknown : ReverseLanResult
    data class Response(val value: ReverseLanResponse) : ReverseLanResult
}

enum class ReverseLanState {
    STOPPED,
    ACTIVATING,
    WAITING_FOR_BRIDGE,
    READY,
    DRAINING,
}

interface ReverseLanTransport {
    val state: kotlinx.coroutines.flow.StateFlow<ReverseLanState>
    suspend fun execute(request: ReverseLanRequest): ReverseLanResult
}

private fun isRelativeApiPath(path: String): Boolean {
    if (!path.startsWith("/api/") || '\r' in path || '\n' in path) return false
    val uri = runCatching { URI(path) }.getOrNull() ?: return false
    return !uri.isAbsolute && uri.host == null && uri.rawQuery == null && uri.fragment == null &&
        uri.rawPath == path
}

private fun requireOnlyKeys(json: JSONObject, allowed: Set<String>) {
    require(json.keys().asSequence().all { it in allowed }) { "unknown reverse LAN field" }
}
