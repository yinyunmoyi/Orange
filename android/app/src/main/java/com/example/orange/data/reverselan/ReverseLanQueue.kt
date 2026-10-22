package com.example.orange.data.reverselan

import com.example.orange.data.logging.AppLog
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.selects.select
import kotlinx.coroutines.withTimeoutOrNull
import org.json.JSONObject

internal class ReverseLanQueue(
    private val responseTimeoutMillis: Long = 22_000L,
    private val completedRetentionMillis: Long = 60_000L,
) {
    private enum class CommandState {
        QUEUED,
        DISPATCHED,
        COMPLETED,
        UNKNOWN,
        CANCELLED,
    }

    private data class Entry(
        val request: ReverseLanRequest,
        val completion: CompletableDeferred<ReverseLanResult>,
        val dispatched: CompletableDeferred<Unit>,
        var state: CommandState,
        var completedResult: ReverseLanResult? = null,
        var resultHash: String? = null,
        var completedAtNanos: Long = 0L,
        val queuedAtNanos: Long = System.nanoTime(),
        var dispatchedAtNanos: Long = 0L,
    )

    enum class CompletionStatus {
        ACCEPTED,
        DUPLICATE,
        CONFLICT,
        NOT_FOUND,
    }

    private val lock = Any()
    private val available = mapOf(
        REVERSE_LAN_INTERACTIVE_LANE to LinkedBlockingQueue<String>(),
        "background" to LinkedBlockingQueue<String>(),
    )
    private val entries = mutableMapOf<String, Entry>()
    private var accepting = true

    suspend fun execute(request: ReverseLanRequest): ReverseLanResult {
        request.validate()
        val completion = CompletableDeferred<ReverseLanResult>()
        synchronized(lock) {
            purgeCompletedLocked()
            if (!accepting) return ReverseLanResult.Unavailable
            require(request.requestId !in entries) { "duplicate reverse LAN request id" }
            entries[request.requestId] = Entry(
                request,
                completion,
                CompletableDeferred(),
                CommandState.QUEUED,
            )
            available.getValue(request.lane).offer(request.requestId)
        }
        logTransition(request, "queued")

        val beforeDispatch = withTimeoutOrNull(responseTimeoutMillis) {
            select<ReverseLanResult?> {
                completion.onAwait { it }
                synchronized(lock) {
                    entries[request.requestId]?.dispatched
                }?.onAwait { null }
            }
        }
        if (beforeDispatch != null) return beforeDispatch
        synchronized(lock) {
            val entry = entries[request.requestId] ?: return ReverseLanResult.DeliveryUnknown
            entry.completedResult?.let { return it }
            when (entry.state) {
                CommandState.QUEUED -> {
                    entry.state = CommandState.CANCELLED
                    entries.remove(request.requestId)
                    logTransition(request, "not_dispatched", elapsedMillis(entry.queuedAtNanos))
                    return ReverseLanResult.NotDispatched
                }
                CommandState.DISPATCHED -> Unit
                CommandState.COMPLETED ->
                    return entry.completedResult ?: ReverseLanResult.DeliveryUnknown
                CommandState.UNKNOWN -> return ReverseLanResult.DeliveryUnknown
                CommandState.CANCELLED -> return ReverseLanResult.NotDispatched
            }
        }
        val afterDispatch = withTimeoutOrNull(responseTimeoutMillis) { completion.await() }
        if (afterDispatch != null) return afterDispatch
        synchronized(lock) {
            val entry = entries[request.requestId] ?: return ReverseLanResult.DeliveryUnknown
            entry.completedResult?.let { return it }
            return when (entry.state) {
                CommandState.QUEUED -> ReverseLanResult.NotDispatched
                CommandState.DISPATCHED -> {
                    entry.state = CommandState.UNKNOWN
                    logTransition(
                        request,
                        "delivery_unknown",
                        elapsedMillis(entry.dispatchedAtNanos),
                    )
                    ReverseLanResult.DeliveryUnknown
                }
                CommandState.COMPLETED ->
                    entry.completedResult ?: ReverseLanResult.DeliveryUnknown
                CommandState.UNKNOWN -> ReverseLanResult.DeliveryUnknown
                CommandState.CANCELLED -> ReverseLanResult.NotDispatched
            }
        }
    }

    fun next(lane: String, waitMillis: Long): ReverseLanRequest? {
        if (lane != REVERSE_LAN_INTERACTIVE_LANE && lane != "background") return null
        val laneQueue = available.getValue(lane)
        val deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(waitMillis.coerceAtLeast(0L))
        while (true) {
            val remainingNanos = deadline - System.nanoTime()
            if (remainingNanos <= 0L) return null
            val requestId = laneQueue.poll(remainingNanos, TimeUnit.NANOSECONDS) ?: return null
            synchronized(lock) {
                val entry = entries[requestId] ?: return@synchronized
                if (entry.state != CommandState.QUEUED) return@synchronized
                entry.state = CommandState.DISPATCHED
                entry.dispatchedAtNanos = System.nanoTime()
                entry.dispatched.complete(Unit)
                logTransition(entry.request, "dispatched", elapsedMillis(entry.queuedAtNanos))
                return entry.request
            }
        }
    }

    fun complete(response: ReverseLanResponse): CompletionStatus {
        response.validate()
        val hash = response.stableHash()
        synchronized(lock) {
            purgeCompletedLocked()
            val entry = entries[response.requestId] ?: run {
                AppLog.event(
                    TAG,
                    "result_not_found",
                    JSONObject().put("requestId", response.requestId),
                    immediateUpload = true,
                )
                return CompletionStatus.NOT_FOUND
            }
            if (entry.state == CommandState.COMPLETED) {
                return if (entry.resultHash == hash) {
                    logTransition(entry.request, "result_duplicate")
                    CompletionStatus.DUPLICATE
                } else {
                    logTransition(entry.request, "result_conflict")
                    CompletionStatus.CONFLICT
                }
            }
            if (entry.state != CommandState.DISPATCHED && entry.state != CommandState.UNKNOWN) {
                logTransition(entry.request, "result_conflict")
                return CompletionStatus.CONFLICT
            }
            entry.state = CommandState.COMPLETED
            val completed = ReverseLanResult.Response(response)
            entry.completedResult = completed
            entry.resultHash = hash
            entry.completedAtNanos = System.nanoTime()
            entry.completion.complete(completed)
            logTransition(
                entry.request,
                "completed",
                elapsedMillis(entry.dispatchedAtNanos),
                response.status,
            )
            return CompletionStatus.ACCEPTED
        }
    }

    fun stop() {
        val pending: List<Pair<CompletableDeferred<ReverseLanResult>, ReverseLanResult>>
        synchronized(lock) {
            accepting = false
            pending = entries.values.mapNotNull { entry ->
                when (entry.state) {
                    CommandState.QUEUED ->
                        entry.completion to ReverseLanResult.NotDispatched
                    CommandState.DISPATCHED, CommandState.UNKNOWN ->
                        entry.completion to ReverseLanResult.DeliveryUnknown
                    CommandState.COMPLETED, CommandState.CANCELLED -> null
                }
            }
            entries.clear()
            available.values.forEach { it.clear() }
        }
        pending.forEach { (completion, result) -> completion.complete(result) }
        if (pending.isNotEmpty()) {
            AppLog.event(
                TAG,
                "queue_stopped",
                JSONObject().put("pendingCount", pending.size),
                immediateUpload = true,
            )
        }
    }

    internal fun pendingCount(): Int = synchronized(lock) {
        entries.values.count { it.state == CommandState.QUEUED || it.state == CommandState.DISPATCHED }
    }

    private fun purgeCompletedLocked() {
        val now = System.nanoTime()
        val retention = TimeUnit.MILLISECONDS.toNanos(completedRetentionMillis)
        entries.entries.removeAll { (_, entry) ->
            entry.state == CommandState.COMPLETED &&
                now - entry.completedAtNanos >= retention
        }
    }

    private fun logTransition(
        request: ReverseLanRequest,
        state: String,
        elapsedMillis: Long? = null,
        status: Int? = null,
    ) {
        AppLog.event(
            TAG,
            "request_$state",
            JSONObject()
                .put("requestId", request.requestId)
                .put("lane", request.lane)
                .put("method", request.method)
                .put("path", request.path)
                .apply {
                    if (elapsedMillis != null) put("elapsedMs", elapsedMillis)
                    if (status != null) put("status", status)
                },
            request.headers.entries
                .firstOrNull { it.key.equals("X-Orange-Debug-Trace-Id", ignoreCase = true) }
                ?.value,
            immediateUpload = true,
        )
    }

    private fun elapsedMillis(startedAtNanos: Long): Long =
        if (startedAtNanos <= 0L) 0L else
            TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - startedAtNanos)

    companion object {
        private const val TAG = "ReverseLan"
    }
}
