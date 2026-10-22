package com.example.orange.data.reverselan

import com.example.orange.data.logging.AppLog
import fi.iki.elonen.NanoHTTPD
import java.io.InputStream
import org.json.JSONArray
import org.json.JSONObject

internal class ReverseLanServer(
    private val serverId: String,
    private val generation: String,
    private val secret: String,
    private val queue: ReverseLanQueue,
    private val onAuthenticatedPoll: () -> Unit,
) : NanoHTTPD(0) {
    val boundPort: Int
        get() = listeningPort

    override fun serve(session: IHTTPSession): Response {
        if (!isAuthorized(session)) {
            AppLog.event(
                TAG,
                "authentication_rejected",
                JSONObject()
                    .put("method", session.method.name)
                    .put("path", session.uri)
                    .put("remote", session.remoteIpAddress.orEmpty().take(100)),
                immediateUpload = true,
            )
            return response(
                Response.Status.UNAUTHORIZED,
                JSONObject().put("code", 401).put("msg", "unauthorized"),
            )
        }
        return runCatching {
            when {
                session.method == Method.GET && session.uri == "/v1/reverse/capabilities" ->
                    response(
                        Response.Status.OK,
                        JSONObject()
                            .put("apiVersion", REVERSE_LAN_API_VERSION)
                            .put("capabilities", JSONArray().put(REVERSE_LAN_CAPABILITY))
                            .put("lanes", JSONArray().put(REVERSE_LAN_INTERACTIVE_LANE)),
                    )
                session.method == Method.GET && session.uri == "/v1/reverse/health" ->
                    response(Response.Status.OK, JSONObject().put("status", "ok"))
                session.method == Method.GET && session.uri == "/v1/reverse/next" ->
                    serveNext(session)
                session.method == Method.POST &&
                    session.uri.startsWith("/v1/reverse/results/") -> serveResult(session)
                else -> response(
                    Response.Status.NOT_FOUND,
                    JSONObject().put("code", 404).put("msg", "not found"),
                )
            }
        }.getOrElse { error ->
            AppLog.event(
                TAG,
                "request_rejected",
                JSONObject()
                    .put("method", session.method.name)
                    .put("path", session.uri)
                    .put("errorType", error.javaClass.name),
                immediateUpload = true,
            )
            response(
                Response.Status.BAD_REQUEST,
                JSONObject().put("code", 400).put("msg", error.message.orEmpty().take(200)),
            )
        }
    }

    private fun serveNext(session: IHTTPSession): Response {
        val lane = session.parameters["lane"]?.firstOrNull().orEmpty()
        val waitMillis = session.parameters["waitMs"]
            ?.firstOrNull()
            ?.toLongOrNull()
            ?.coerceIn(0L, MAX_POLL_WAIT_MILLIS)
            ?: 0L
        if (lane != REVERSE_LAN_INTERACTIVE_LANE) {
            return response(
                Response.Status.BAD_REQUEST,
                JSONObject().put("code", 400).put("msg", "unsupported lane"),
            )
        }
        onAuthenticatedPoll()
        val request = queue.next(lane, waitMillis)
            ?: return newFixedLengthResponse(Response.Status.NO_CONTENT, MIME_PLAINTEXT, "")
        return response(Response.Status.OK, request.toJson())
    }

    private fun serveResult(session: IHTTPSession): Response {
        val contentLength = session.headers["content-length"]?.toLongOrNull()
        if (
            contentLength == null ||
            contentLength < 0L ||
            contentLength > MAX_RESULT_ENVELOPE_BYTES
        ) {
            return response(
                Response.Status.PAYLOAD_TOO_LARGE,
                JSONObject().put("code", 413).put("msg", "result too large"),
            )
        }
        val raw = readUtf8Body(
            session.inputStream,
            contentLength.toInt(),
            MAX_RESULT_ENVELOPE_BYTES,
        )
        val result = ReverseLanResponse.fromJson(JSONObject(raw))
        val requestId = session.uri.removePrefix("/v1/reverse/results/")
        require(requestId == result.requestId) { "request id mismatch" }
        return when (queue.complete(result)) {
            ReverseLanQueue.CompletionStatus.ACCEPTED,
            ReverseLanQueue.CompletionStatus.DUPLICATE,
            -> newFixedLengthResponse(Response.Status.NO_CONTENT, MIME_PLAINTEXT, "")
            ReverseLanQueue.CompletionStatus.CONFLICT ->
                response(
                    Response.Status.CONFLICT,
                    JSONObject().put("code", 409).put("msg", "conflicting result"),
                )
            ReverseLanQueue.CompletionStatus.NOT_FOUND ->
                response(
                    Response.Status.NOT_FOUND,
                    JSONObject().put("code", 404).put("msg", "request not found"),
                )
        }
    }

    private fun isAuthorized(session: IHTTPSession): Boolean =
        constantTimeEquals(session.headers["authorization"], "Bearer $secret") &&
            constantTimeEquals(session.headers[REVERSE_LAN_SERVER_HEADER.lowercase()], serverId) &&
            constantTimeEquals(
                session.headers[REVERSE_LAN_GENERATION_HEADER.lowercase()],
                generation,
            )

    private fun response(status: Response.Status, json: JSONObject): Response =
        newFixedLengthResponse(status, "application/json; charset=utf-8", json.toString())

    companion object {
        private const val TAG = "ReverseLan"
        private const val MAX_POLL_WAIT_MILLIS = 25_000L
        private const val MAX_RESULT_ENVELOPE_BYTES =
            REVERSE_LAN_MAX_RESULT_BODY_BYTES + (64 * 1024)

        fun constantTimeEquals(actual: String?, expected: String): Boolean {
            val left = actual?.toByteArray(Charsets.UTF_8) ?: ByteArray(0)
            val right = expected.toByteArray(Charsets.UTF_8)
            var difference = left.size xor right.size
            val length = maxOf(left.size, right.size)
            for (index in 0 until length) {
                val leftByte = if (index < left.size) left[index].toInt() else 0
                val rightByte = if (index < right.size) right[index].toInt() else 0
                difference = difference or (leftByte xor rightByte)
            }
            return difference == 0
        }

        internal fun readUtf8Body(
            input: InputStream,
            contentLength: Int,
            maxBytes: Int,
        ): String {
            require(contentLength in 0..maxBytes) { "result too large" }
            val bytes = ByteArray(contentLength)
            var offset = 0
            while (offset < bytes.size) {
                val count = input.read(bytes, offset, bytes.size - offset)
                require(count > 0) { "incomplete result body" }
                offset += count
            }
            return bytes.toString(Charsets.UTF_8)
        }
    }
}
